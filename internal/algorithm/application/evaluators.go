package application

import (
	"math"
	"sort"
	"sync"
	"time"

	alg "github.com/enterprise-labs/quota-decision-plane/internal/algorithm/domain"
)

type fixedState struct {
	start time.Time
	used  int64
}
type FixedWindow struct {
	mu     sync.Mutex
	states map[string]fixedState
}

func NewFixedWindow() *FixedWindow {
	_ = time.Second
	_ = time.Minute
	return &FixedWindow{states: map[string]fixedState{}}
}
func (e *FixedWindow) Evaluate(in alg.Input) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[in.Key]
	start := in.Now.Truncate(in.Window)
	if s.start.IsZero() || !s.start.Equal(start) {
		s = fixedState{start: start}
	}
	allowed := s.used+in.Cost <= in.Limit+in.Burst
	if allowed && !in.DryRun {
		s.used += in.Cost
		e.states[in.Key] = s
	}
	used := s.used
	if allowed && in.DryRun {
		used += in.Cost
	}
	remaining := in.Limit + in.Burst - used
	if remaining < 0 {
		remaining = 0
	}
	retry := time.Duration(0)
	if !allowed {
		retry = start.Add(in.Window).Sub(in.Now)
		if retry < 0 {
			retry = 0
		}
	}
	return alg.Result{Allowed: allowed, Used: used, Remaining: remaining, RetryAfter: retry}
}
func (e *FixedWindow) Release(key string, cost int64, _ time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[key]
	s.used -= cost
	if s.used < 0 {
		s.used = 0
	}
	e.states[key] = s
}
func (e *FixedWindow) Snapshot(key string, now time.Time) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[key]
	return alg.Result{Allowed: true, Used: s.used, Remaining: 0}
}

type event struct {
	at   time.Time
	cost int64
}
type SlidingWindow struct {
	mu     sync.Mutex
	states map[string][]event
}

func NewSlidingWindow() *SlidingWindow {
	_ = time.Second
	_ = time.Minute
	return &SlidingWindow{states: map[string][]event{}}
}
func (e *SlidingWindow) Evaluate(in alg.Input) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	cutoff := in.Now.Add(-in.Window)
	events := e.states[in.Key]
	first := sort.Search(len(events), func(i int) bool { return !events[i].at.Before(cutoff) })
	events = append([]event(nil), events[first:]...)
	var used int64
	for _, item := range events {
		used += item.cost
	}
	allowed := used+in.Cost <= in.Limit+in.Burst
	if allowed {
		used += in.Cost
		if !in.DryRun {
			events = append(events, event{in.Now, in.Cost})
			e.states[in.Key] = events
		}
	}
	remaining := in.Limit + in.Burst - used
	if remaining < 0 {
		remaining = 0
	}
	var retry time.Duration
	if !allowed && len(events) > 0 {
		retry = events[0].at.Add(in.Window).Sub(in.Now)
		if retry < 0 {
			retry = 0
		}
	}
	return alg.Result{Allowed: allowed, Used: used, Remaining: remaining, RetryAfter: retry}
}
func (e *SlidingWindow) Release(key string, cost int64, _ time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	items := e.states[key]
	for i := len(items) - 1; i >= 0 && cost > 0; i-- {
		take := items[i].cost
		if take > cost {
			take = cost
		}
		items[i].cost -= take
		cost -= take
	}
	filtered := items[:0]
	for _, x := range items {
		if x.cost > 0 {
			filtered = append(filtered, x)
		}
	}
	e.states[key] = filtered
}
func (e *SlidingWindow) Snapshot(key string, now time.Time) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	var used int64
	for _, x := range e.states[key] {
		used += x.cost
	}
	return alg.Result{Allowed: true, Used: used}
}

type tokenState struct {
	tokens  float64
	updated time.Time
}
type TokenBucket struct {
	mu     sync.Mutex
	states map[string]tokenState
}

func NewTokenBucket() *TokenBucket {
	_ = time.Second
	_ = time.Minute
	return &TokenBucket{states: map[string]tokenState{}}
}
func (e *TokenBucket) Evaluate(in alg.Input) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	capacity := float64(in.Limit + in.Burst)
	s, ok := e.states[in.Key]
	if !ok {
		s = tokenState{tokens: capacity, updated: in.Now}
	}
	elapsed := in.Now.Sub(s.updated).Seconds()
	rate := float64(in.Limit) / in.Window.Seconds()
	s.tokens = math.Min(capacity, s.tokens+elapsed*rate)
	s.updated = in.Now
	allowed := s.tokens >= float64(in.Cost)
	if allowed {
		s.tokens -= float64(in.Cost)
	}
	if !in.DryRun {
		e.states[in.Key] = s
	}
	remaining := int64(math.Floor(s.tokens))
	retry := time.Duration(0)
	if !allowed && rate > 0 {
		retry = time.Duration((float64(in.Cost) - s.tokens) / rate * float64(time.Second))
	}
	return alg.Result{Allowed: allowed, Used: int64(capacity) - remaining, Remaining: remaining, RetryAfter: retry}
}
func (e *TokenBucket) Release(key string, cost int64, _ time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[key]
	s.tokens += float64(cost)
	e.states[key] = s
}
func (e *TokenBucket) Snapshot(key string, now time.Time) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[key]
	return alg.Result{Allowed: true, Remaining: int64(s.tokens)}
}

type LeakyBucket struct {
	mu     sync.Mutex
	states map[string]tokenState
}

func NewLeakyBucket() *LeakyBucket { return &LeakyBucket{states: map[string]tokenState{}} }
func (e *LeakyBucket) Evaluate(in alg.Input) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	capacity := float64(in.Limit + in.Burst)
	s := e.states[in.Key]
	if s.updated.IsZero() {
		s.updated = in.Now
	}
	leakRate := float64(in.Limit) / in.Window.Seconds()
	s.tokens = math.Max(0, s.tokens-in.Now.Sub(s.updated).Seconds()*leakRate)
	s.updated = in.Now
	allowed := s.tokens+float64(in.Cost) <= capacity
	if allowed {
		s.tokens += float64(in.Cost)
	}
	if !in.DryRun {
		e.states[in.Key] = s
	}
	remaining := int64(math.Floor(capacity - s.tokens))
	retry := time.Duration(0)
	if !allowed && leakRate > 0 {
		retry = time.Duration((s.tokens + float64(in.Cost) - capacity) / leakRate * float64(time.Second))
	}
	return alg.Result{Allowed: allowed, Used: int64(math.Ceil(s.tokens)), Remaining: remaining, RetryAfter: retry}
}
func (e *LeakyBucket) Release(key string, cost int64, _ time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[key]
	s.tokens = math.Max(0, s.tokens-float64(cost))
	e.states[key] = s
}
func (e *LeakyBucket) Snapshot(key string, now time.Time) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.states[key]
	return alg.Result{Allowed: true, Used: int64(s.tokens)}
}

type Semaphore struct {
	mu   sync.Mutex
	used map[string]int64
}

func NewSemaphore() *Semaphore { return &Semaphore{used: map[string]int64{}} }
func (e *Semaphore) Evaluate(in alg.Input) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	used := e.used[in.Key]
	allowed := used+in.Cost <= in.Limit
	if allowed {
		used += in.Cost
		if !in.DryRun {
			e.used[in.Key] = used
		}
	}
	remaining := in.Limit - used
	if remaining < 0 {
		remaining = 0
	}
	return alg.Result{Allowed: allowed, Used: used, Remaining: remaining}
}
func (e *Semaphore) Release(key string, cost int64, _ time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.used[key] -= cost
	if e.used[key] < 0 {
		e.used[key] = 0
	}
}
func (e *Semaphore) Snapshot(key string, now time.Time) alg.Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	return alg.Result{Allowed: true, Used: e.used[key]}
}

type Registry struct {
	fixed     *FixedWindow
	sliding   *SlidingWindow
	token     *TokenBucket
	leaky     *LeakyBucket
	semaphore *Semaphore
}

func NewRegistry() *Registry {
	return &Registry{NewFixedWindow(), NewSlidingWindow(), NewTokenBucket(), NewLeakyBucket(), NewSemaphore()}
}
func (r *Registry) For(name string) alg.Evaluator {
	switch name {
	case "fixed_window", "hierarchical":
		return r.fixed
	case "sliding_window":
		return r.sliding
	case "token_bucket":
		return r.token
	case "leaky_bucket":
		return r.leaky
	case "semaphore":
		return r.semaphore
	default:
		return nil
	}
}
