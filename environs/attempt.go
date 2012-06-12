package environs

import (
	"time"
)

// AttemptStrategy represents a strategy for waiting for an action
// to complete successfully.  
type AttemptStrategy struct {
	Total time.Duration // total duration of attempt.
	Delay time.Duration // interval between each try in the burst.
}

type attempt struct {
	AttemptStrategy
	end time.Time
}

// Start begins a new sequence of attempts with the given strategy.
func (a AttemptStrategy) Start() *attempt {
	return &attempt{
		AttemptStrategy: a,
	}
}

// Next waits until it is time to perform the next attempt or returns
// false if not attempts remain.
func (a *attempt) Next() bool {
	now := time.Now()
	// we always make at least one attempt.
	if a.end.IsZero() {
		a.end = now.Add(a.Total)
		return true
	}

	if !now.Add(a.Delay).Before(a.end) {
		return false
	}
	time.Sleep(a.Delay)
	return true
}
