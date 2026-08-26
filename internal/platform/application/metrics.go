package application

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	requests  atomic.Uint64
	errors    atomic.Uint64
	decisions atomic.Uint64
	denied    atomic.Uint64
	latencyNS atomic.Uint64
	mu        sync.RWMutex
	byPath    map[string]uint64
	started   time.Time
}

func NewMetrics() *Metrics { return &Metrics{byPath: map[string]uint64{}, started: time.Now()} }
func (m *Metrics) Observe(path string, status int, duration time.Duration) {
	m.requests.Add(1)
	m.latencyNS.Add(uint64(duration))
	if status >= 400 {
		m.errors.Add(1)
	}
	m.mu.Lock()
	m.byPath[path]++
	m.mu.Unlock()
}
func (m *Metrics) Decision(allowed bool) {
	m.decisions.Add(1)
	if !allowed {
		m.denied.Add(1)
	}
}
func (m *Metrics) WritePrometheus(w io.Writer, dropped uint64) {
	requests := m.requests.Load()
	average := float64(0)
	if requests > 0 {
		average = float64(m.latencyNS.Load()) / float64(requests) / 1e9
	}
	lines := []string{"# HELP quota_http_requests_total Total HTTP requests.", "# TYPE quota_http_requests_total counter", fmt.Sprintf("quota_http_requests_total %d", requests), fmt.Sprintf("quota_http_errors_total %d", m.errors.Load()), fmt.Sprintf("quota_decisions_total %d", m.decisions.Load()), fmt.Sprintf("quota_decisions_denied_total %d", m.denied.Load()), fmt.Sprintf("quota_http_average_duration_seconds %.6f", average), fmt.Sprintf("quota_audit_dropped_total %d", dropped), fmt.Sprintf("quota_uptime_seconds %.0f", time.Since(m.started).Seconds())}
	m.mu.RLock()
	paths := make([]string, 0, len(m.byPath))
	for path := range m.byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		escaped := strings.ReplaceAll(path, "\"", "\\\"")
		lines = append(lines, fmt.Sprintf("quota_http_path_requests_total{path=\"%s\"} %d", escaped, m.byPath[path]))
	}
	m.mu.RUnlock()
	_, _ = io.WriteString(w, strings.Join(lines, "\n")+"\n")
}
