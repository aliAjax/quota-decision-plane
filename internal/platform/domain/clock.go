package domain

import "time"

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type ManualClock struct{ Current time.Time }

func (c *ManualClock) Now() time.Time          { return c.Current }
func (c *ManualClock) Advance(d time.Duration) { c.Current = c.Current.Add(d) }
