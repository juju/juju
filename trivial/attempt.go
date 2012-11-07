package trivial

import (
	"time"
)

// AttemptStrategy represents a strategy for waiting for an action
// to complete successfully.
type AttemptStrategy struct {
	Total time.Duration // total duration of attempt.
	Delay time.Duration // interval between each try in the burst.
}

type Attempt struct {
	strategy AttemptStrategy
	end      time.Time
}

// Start begins a new sequence of attempts for the given strategy.
func (a AttemptStrategy) Start() *Attempt {
	return &Attempt{
		strategy: a,
	}
}

// Next waits until it is time to perform the next attempt or returns
// false if it is time to stop trying.
func (a *Attempt) Next() bool {
	now := time.Now()
	// we always make at least one attempt.
	if a.end.IsZero() {
		a.end = now.Add(a.strategy.Total)
		return true
	}

	if !now.Add(a.strategy.Delay).Before(a.end) {
		return false
	}
	time.Sleep(a.strategy.Delay)
	return true
}
