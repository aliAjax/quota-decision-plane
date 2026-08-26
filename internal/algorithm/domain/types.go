package domain

import "time"

type Input struct {
	Key    string
	Limit  int64
	Burst  int64
	Cost   int64
	Window time.Duration
	Now    time.Time
	DryRun bool
}

type Result struct {
	Allowed    bool
	Used       int64
	Remaining  int64
	RetryAfter time.Duration
}

type Evaluator interface {
	Evaluate(Input) Result
	Release(key string, cost int64, now time.Time)
	Snapshot(key string, now time.Time) Result
}
