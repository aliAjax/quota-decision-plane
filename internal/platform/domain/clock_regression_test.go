package domain

import (
	"testing"
	"time"
)

func TestManualClockAdvanceMovesForward(t *testing.T) {
	clock := &ManualClock{Current: time.Unix(100, 0)}
	clock.Advance(5 * time.Second)
	if got := clock.Now(); !got.After(time.Unix(100, 0)) {
		t.Fatalf("clock did not move forward: %v", got)
	}
}
