package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	algapp "github.com/enterprise-labs/quota-decision-plane/internal/algorithm/application"
	alg "github.com/enterprise-labs/quota-decision-plane/internal/algorithm/domain"
	auditapp "github.com/enterprise-labs/quota-decision-plane/internal/audit/application"
	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
	cfgapp "github.com/enterprise-labs/quota-decision-plane/internal/configuration/application"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	resapp "github.com/enterprise-labs/quota-decision-plane/internal/reservation/application"
	reservation "github.com/enterprise-labs/quota-decision-plane/internal/reservation/domain"
	shardapp "github.com/enterprise-labs/quota-decision-plane/internal/shard/application"
)

type Clock interface{ Now() time.Time }
type Engine struct {
	config       *cfgapp.Service
	matcher      *Matcher
	algorithms   *algapp.Registry
	reservations *resapp.Service
	audit        *auditapp.Bus
	clock        Clock
	epoch        func(string) uint64
}

func NewEngine(config *cfgapp.Service, matcher *Matcher, algorithms *algapp.Registry, audit *auditapp.Bus, clock Clock, epoch func(string) uint64) *Engine {
	return &Engine{config: config, matcher: matcher, algorithms: algorithms, audit: audit, clock: clock, epoch: epoch}
}
func (e *Engine) SetReservations(service *resapp.Service) { e.reservations = service }
func randomID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + hex.EncodeToString(raw[:])
}
func (e *Engine) evaluate(def quota.Definition, request quota.DecisionRequest, dryRun bool, version int64) quota.Decision {
	key := def.ID + "/" + request.ScopeKey()
	evaluator := e.algorithms.For(string(def.Algorithm))
	if evaluator == nil {
		return quota.Decision{Allowed: false, Reason: "unsupported_algorithm", QuotaID: def.ID, ConfigVersion: version}
	}
	result := evaluator.Evaluate(alg.Input{Key: key, Limit: def.Limit, Burst: def.Burst, Cost: request.Cost, Window: def.Window, Now: e.clock.Now(), DryRun: dryRun})
	mode := def.Mode
	if mode == "" {
		mode = "strong"
	}
	decision := quota.Decision{Allowed: result.Allowed, QuotaID: def.ID, Limit: def.Limit + def.Burst, Used: result.Used, Remaining: result.Remaining, RetryAfterMS: result.RetryAfter.Milliseconds(), ConfigVersion: version, FencingEpoch: e.epoch(key), Mode: mode}
	if result.Allowed {
		decision.Reason = "within_quota"
	} else {
		decision.Reason = "quota_exceeded"
	}
	return decision
}
func (e *Engine) Check(ctx context.Context, request quota.DecisionRequest, consume bool) (quota.Decision, error) {
	if err := ctx.Err(); err != nil {
		return quota.Decision{}, err
	}
	if err := request.Validate(false); err != nil {
		return quota.Decision{}, err
	}
	active, err := e.config.Active(ctx)
	if err != nil {
		return quota.Decision{}, fmt.Errorf("load configuration: %w", err)
	}
	def, err := e.matcher.Match(active.Definitions, request)
	if err != nil {
		return quota.Decision{Allowed: false, Reason: "quota_not_found", ConfigVersion: active.Version}, fmt.Errorf("match quota: %w", err)
	}
	chain, err := e.matcher.Chain(active.Definitions, def)
	if err != nil {
		return quota.Decision{}, fmt.Errorf("resolve hierarchy: %w", err)
	}
	accepted := make([]struct {
		def quota.Definition
		key string
	}, 0, len(chain))
	var decision quota.Decision
	var leafDecision quota.Decision
	for i, item := range chain {
		decision = e.evaluate(item, request, !consume, active.Version)
		if i == 0 {
			leafDecision = decision
		}
		if !decision.Allowed {
			if consume {
				for _, a := range accepted {
					e.algorithms.For(string(a.def.Algorithm)).Release(a.key, request.Cost, e.clock.Now())
				}
			}
			break
		}
		accepted = append(accepted, struct {
			def quota.Definition
			key string
		}{item, item.ID + "/" + request.ScopeKey()})
	}
	if decision.Allowed {
		decision = leafDecision
		for _, acceptedItem := range accepted {
			decision.Allocations = append(decision.Allocations, quota.Allocation{
				CounterKey: acceptedItem.key,
				Algorithm:  acceptedItem.def.Algorithm,
				Cost:       request.Cost,
			})
		}
	}
	if shadow := e.config.Shadow(); shadow != nil {
		if shadowDef, matchErr := e.matcher.Match(shadow.Definitions, request); matchErr == nil {
			d := e.evaluate(shadowDef, request, true, shadow.Version)
			decision.Shadow = &quota.ShadowDecision{Allowed: d.Allowed, QuotaID: d.QuotaID, Reason: d.Reason}
		}
	}
	outcome := "denied"
	if decision.Allowed {
		outcome = "allowed"
	}
	e.audit.Publish(audit.Event{ID: randomID("evt_"), Type: "decision.check", TenantID: request.TenantID, Resource: request.Resource, SubjectID: decision.QuotaID, Outcome: outcome, RequestID: request.RequestID, CreatedAt: e.clock.Now()})
	return decision, nil
}
func (e *Engine) Reserve(ctx context.Context, request quota.DecisionRequest) (quota.Decision, error) {
	if err := request.Validate(true); err != nil {
		return quota.Decision{}, err
	}
	decision, err := e.Check(ctx, request, true)
	if err != nil || !decision.Allowed {
		return decision, err
	}
	ttl := time.Duration(request.TTLMillis) * time.Millisecond
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	id := randomID("rsv_")
	counterKey := decision.QuotaID + "/" + request.ScopeKey()
	item := reservation.Reservation{ID: id, IdempotencyKey: request.IdempotencyKey, TenantID: request.TenantID, Resource: request.Resource, QuotaID: decision.QuotaID, CounterKey: counterKey, Cost: request.Cost, ExpiresAt: e.clock.Now().Add(ttl), FencingEpoch: decision.FencingEpoch, Allocations: decision.Allocations}
	if e.reservations == nil {
		return quota.Decision{}, errors.New("reservation service unavailable")
	}
	if err = e.reservations.Create(ctx, item); err != nil {
		e.Release(decision.Allocations)
		return quota.Decision{}, fmt.Errorf("create reservation: %w", err)
	}
	decision.ReservationID = id
	e.audit.Publish(audit.Event{ID: randomID("evt_"), Type: "reservation.created", TenantID: request.TenantID, Resource: request.Resource, SubjectID: id, Outcome: "pending", CreatedAt: e.clock.Now()})
	return decision, nil
}
func (e *Engine) Commit(ctx context.Context, id string) (reservation.Reservation, error) {
	item, err := e.reservations.Commit(ctx, id)
	if err == nil {
		e.audit.Publish(audit.Event{ID: randomID("evt_"), Type: "reservation.committed", TenantID: item.TenantID, Resource: item.Resource, SubjectID: id, Outcome: "committed", CreatedAt: e.clock.Now()})
	}
	return item, err
}
func (e *Engine) Cancel(ctx context.Context, id string) (reservation.Reservation, error) {
	item, err := e.reservations.Cancel(ctx, id)
	if err == nil {
		e.audit.Publish(audit.Event{ID: randomID("evt_"), Type: "reservation.cancelled", TenantID: item.TenantID, Resource: item.Resource, SubjectID: id, Outcome: "cancelled", CreatedAt: e.clock.Now()})
	}
	return item, err
}
func (e *Engine) BatchCheck(ctx context.Context, requests []quota.DecisionRequest, consume bool) ([]quota.Decision, error) {
	if len(requests) == 0 {
		return nil, errors.New("batch is empty")
	}
	if len(requests) > 100 {
		return nil, errors.New("batch exceeds 100 items")
	}
	results := make([]quota.Decision, 0, len(requests))
	for i, r := range requests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, err := e.Check(ctx, r, consume)
		if err != nil {
			return nil, fmt.Errorf("batch item %d: %w", i, err)
		}
		results = append(results, d)
	}
	return results, nil
}
func (e *Engine) Release(allocations []quota.Allocation) {
	for _, allocation := range allocations {
		if evaluator := e.algorithms.For(string(allocation.Algorithm)); evaluator != nil {
			evaluator.Release(allocation.CounterKey, allocation.Cost, e.clock.Now())
		}
	}
}
func DefaultEpoch(key string) uint64 { return shardapp.HashUint64(key)%1000000 + 1 }
