package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
	cfgapp "github.com/enterprise-labs/quota-decision-plane/internal/configuration/application"
	cfg "github.com/enterprise-labs/quota-decision-plane/internal/configuration/domain"
	idemapp "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/application"
	idem "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/domain"
	leaseapp "github.com/enterprise-labs/quota-decision-plane/internal/lease/application"
	platform "github.com/enterprise-labs/quota-decision-plane/internal/platform/application"
	quotaapp "github.com/enterprise-labs/quota-decision-plane/internal/quota/application"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	reservation "github.com/enterprise-labs/quota-decision-plane/internal/reservation/domain"
	shardapp "github.com/enterprise-labs/quota-decision-plane/internal/shard/application"
)

type API struct {
	engine       *quotaapp.Engine
	config       *cfgapp.Service
	idempotency  *idemapp.Service
	audit        audit.Sink
	ring         *shardapp.Ring
	leases       *leaseapp.Manager
	metrics      *platform.Metrics
	auditDropped func() uint64
	logger       *slog.Logger
	ready        func() bool
	idemMu       sync.Mutex
}
type Dependencies struct {
	Engine        *quotaapp.Engine
	Configuration *cfgapp.Service
	Idempotency   *idemapp.Service
	Audit         audit.Sink
	Ring          *shardapp.Ring
	Leases        *leaseapp.Manager
	Metrics       *platform.Metrics
	AuditDropped  func() uint64
	Logger        *slog.Logger
	Ready         func() bool
}

func NewAPI(d Dependencies) *API {
	return &API{engine: d.Engine, config: d.Configuration, idempotency: d.Idempotency, audit: d.Audit, ring: d.Ring, leases: d.Leases, metrics: d.Metrics, auditDropped: d.AuditDropped, logger: d.Logger, ready: d.Ready}
}
func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /readyz", a.readiness)
	mux.HandleFunc("GET /metrics", a.prometheus)
	mux.HandleFunc("POST /v1/check", a.check)
	mux.HandleFunc("POST /v1/reserve", a.reserve)
	mux.HandleFunc("POST /v1/batch-check", a.batchCheck)
	mux.HandleFunc("POST /v1/reservations/{id}/commit", a.commit)
	mux.HandleFunc("POST /v1/reservations/{id}/cancel", a.cancel)
	mux.HandleFunc("GET /v1/quotas", a.activeQuotas)
	mux.HandleFunc("GET /v1/config/versions", a.versions)
	mux.HandleFunc("POST /v1/config/drafts", a.createDraft)
	mux.HandleFunc("POST /v1/config/drafts/{id}/validate", a.validateDraft)
	mux.HandleFunc("POST /v1/config/drafts/{id}/publish", a.publishDraft)
	mux.HandleFunc("POST /v1/config/rollback", a.rollback)
	mux.HandleFunc("POST /v1/config/shadow", a.shadow)
	mux.HandleFunc("GET /v1/audit/events", a.auditEvents)
	mux.HandleFunc("GET /v1/shards/{key}", a.locateShard)
	mux.HandleFunc("GET /v1/leases", a.listLeases)
	return mux
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]any{"error": code, "message": err.Error()})
}
func decodeJSON(r *http.Request, target any) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("request body is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return body, fmt.Errorf("decode JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return body, errors.New("multiple JSON values are not allowed")
	}
	return body, nil
}
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "healthy", "time": time.Now().UTC()})
}
func (a *API) readiness(w http.ResponseWriter, r *http.Request) {
	if a.ready != nil && !a.ready() {
		writeError(w, http.StatusServiceUnavailable, "not_ready", errors.New("configuration is unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
func (a *API) prometheus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	dropped := uint64(0)
	if a.auditDropped != nil {
		dropped = a.auditDropped()
	}
	a.metrics.WritePrometheus(w, dropped)
}
func (a *API) idempotent(ctx context.Context, w http.ResponseWriter, route, key string, body []byte, fn func() (int, any, error)) {
	a.idemMu.Lock()
	defer a.idemMu.Unlock()
	if strings.TrimSpace(key) == "" {
		writeError(w, http.StatusBadRequest, "idempotency_required", errors.New("idempotency_key is required"))
		return
	}
	scope := route + ":" + key
	record, replay, err := a.idempotency.Replay(ctx, scope, body)
	if errors.Is(err, idem.ErrConflict) {
		writeError(w, http.StatusConflict, "idempotency_conflict", err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "idempotency_failure", err)
		return
	}
	if replay {
		w.Header().Set("X-Idempotent-Replay", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(record.Status)
		_, _ = w.Write(record.Body)
		return
	}
	status, value, err := fn()
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	response, err := json.Marshal(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encoding_failure", err)
		return
	}
	response = append(response, '\n')
	if err = a.idempotency.Save(ctx, scope, body, response, status); err != nil {
		writeError(w, http.StatusInternalServerError, "idempotency_failure", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}
func (a *API) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reservation.ErrNotFound), errors.Is(err, cfg.ErrDraftNotFound), errors.Is(err, cfg.ErrVersionNotFound):
		writeError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, reservation.ErrInvalidTransition), errors.Is(err, cfg.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err)
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", err)
	}
}
func (a *API) check(w http.ResponseWriter, r *http.Request) {
	var request quota.DecisionRequest
	body, err := decodeJSON(r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	a.idempotent(r.Context(), w, "check", request.IdempotencyKey, body, func() (int, any, error) {
		decision, err := a.engine.Check(r.Context(), request, true)
		if err == nil {
			a.metrics.Decision(decision.Allowed)
		}
		return http.StatusOK, decision, err
	})
}
func (a *API) reserve(w http.ResponseWriter, r *http.Request) {
	var request quota.DecisionRequest
	body, err := decodeJSON(r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	a.idempotent(r.Context(), w, "reserve", request.IdempotencyKey, body, func() (int, any, error) {
		decision, err := a.engine.Reserve(r.Context(), request)
		if err == nil {
			a.metrics.Decision(decision.Allowed)
		}
		return http.StatusCreated, decision, err
	})
}

type batchRequest struct {
	IdempotencyKey string                  `json:"idempotency_key"`
	Consume        bool                    `json:"consume"`
	Requests       []quota.DecisionRequest `json:"requests"`
}

func (a *API) batchCheck(w http.ResponseWriter, r *http.Request) {
	var request batchRequest
	body, err := decodeJSON(r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	a.idempotent(r.Context(), w, "batch", request.IdempotencyKey, body, func() (int, any, error) {
		decisions, err := a.engine.BatchCheck(r.Context(), request.Requests, request.Consume)
		if err == nil {
			for _, d := range decisions {
				a.metrics.Decision(d.Allowed)
			}
		}
		return http.StatusOK, map[string]any{"decisions": decisions, "count": len(decisions)}, err
	})
}

type actionRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

func (a *API) commit(w http.ResponseWriter, r *http.Request) {
	var request actionRequest
	body, err := decodeJSON(r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	id := r.PathValue("id")
	a.idempotent(r.Context(), w, "commit:"+id, request.IdempotencyKey, body, func() (int, any, error) {
		item, err := a.engine.Commit(r.Context(), id)
		return http.StatusOK, item, err
	})
}
func (a *API) cancel(w http.ResponseWriter, r *http.Request) {
	var request actionRequest
	body, err := decodeJSON(r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	id := r.PathValue("id")
	a.idempotent(r.Context(), w, "cancel:"+id, request.IdempotencyKey, body, func() (int, any, error) {
		item, err := a.engine.Cancel(r.Context(), id)
		return http.StatusOK, item, err
	})
}
func (a *API) activeQuotas(w http.ResponseWriter, r *http.Request) {
	version, err := a.config.Active(r.Context())
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}
func (a *API) versions(w http.ResponseWriter, r *http.Request) {
	versions, err := a.config.Versions(r.Context())
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}
func (a *API) createDraft(w http.ResponseWriter, r *http.Request) {
	var draft cfg.Draft
	if _, err := decodeJSON(r, &draft); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	created, err := a.config.CreateDraft(r.Context(), draft)
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
func (a *API) validateDraft(w http.ResponseWriter, r *http.Request) {
	result, err := a.config.Validate(r.Context(), r.PathValue("id"))
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *API) publishDraft(w http.ResponseWriter, r *http.Request) {
	version, err := a.config.Publish(r.Context(), r.PathValue("id"))
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}

type versionRequest struct {
	Version int64 `json:"version"`
}

func (a *API) rollback(w http.ResponseWriter, r *http.Request) {
	var request versionRequest
	if _, err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	version, err := a.config.Rollback(r.Context(), request.Version)
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}
func (a *API) shadow(w http.ResponseWriter, r *http.Request) {
	var request versionRequest
	if _, err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	if err := a.config.SetShadow(r.Context(), request.Version); err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shadow_version": request.Version})
}
func (a *API) auditEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	events, err := a.audit.List(r.Context(), limit)
	if err != nil {
		a.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "count": len(events)})
}
func (a *API) locateShard(w http.ResponseWriter, r *http.Request) {
	assignment, ok := a.ring.Locate(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "ring_empty", errors.New("no healthy shard nodes"))
		return
	}
	writeJSON(w, http.StatusOK, assignment)
}
func (a *API) listLeases(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"leases": a.leases.List()})
}
