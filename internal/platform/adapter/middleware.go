package adapter

import (
	"context"
	"crypto/subtle"
	"fmt"
	platform "github.com/enterprise-labs/quota-decision-plane/internal/platform/application"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

type Middleware struct {
	config   platform.Config
	logger   *slog.Logger
	metrics  *platform.Metrics
	mu       sync.Mutex
	visitors map[string]*visitor
}
type visitor struct {
	tokens  float64
	updated time.Time
}

func NewMiddleware(cfg platform.Config, logger *slog.Logger, metrics *platform.Metrics) *Middleware {
	return &Middleware{config: cfg, logger: logger, metrics: metrics, visitors: map[string]*visitor{}}
}
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	handler := m.recover(next)
	handler = m.timeout(handler)
	handler = m.bodyLimit(handler)
	handler = m.authenticate(handler)
	handler = m.rateLimit(handler)
	handler = m.requestID(handler)
	handler = m.observe(handler)
	return handler
}
func (m *Middleware) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)
		m.metrics.Observe(r.URL.Path, rec.status, duration)
		requestID := rec.Header().Get("X-Request-ID")
		m.logger.InfoContext(r.Context(), "request completed", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", duration, "request_id", requestID)
	})
}
func (m *Middleware) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = "req-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}
func (m *Middleware) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		now := time.Now()
		m.mu.Lock()
		v := m.visitors[host]
		if v == nil {
			v = &visitor{tokens: float64(m.config.RateLimit), updated: now}
			m.visitors[host] = v
		}
		elapsed := now.Sub(v.updated).Seconds()
		v.tokens += elapsed * float64(m.config.RateLimit)
		if v.tokens > float64(m.config.RateLimit) {
			v.tokens = float64(m.config.RateLimit)
		}
		v.updated = now
		allowed := v.tokens >= 1
		if allowed {
			v.tokens--
		}
		m.mu.Unlock()
		if !allowed {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (m *Middleware) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		provided := r.Header.Get("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(m.config.APIKey)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (m *Middleware) bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, m.config.MaxBodyBytes)
		next.ServeHTTP(w, r)
	})
}
func (m *Middleware) timeout(next http.Handler) http.Handler {
	return http.TimeoutHandler(next, m.config.RequestTimeout, "request timeout")
}
func (m *Middleware) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				m.logger.Error("panic recovered", "error", fmt.Sprint(value))
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type requestIDKey struct{}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}
