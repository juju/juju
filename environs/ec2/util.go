package ec2

import (
	"time"
)

// attemptStrategy represents a strategy for waiting for an ec2 request
// to complete successfully.  
// 
type attemptStrategy struct {
	total time.Duration // total duration of attempt.
	delay time.Duration // interval between each try in the burst.
}

type attempt struct {
	attemptStrategy
	end time.Time
}

func (a attemptStrategy) start() *attempt {
	return &attempt{
		attemptStrategy: a,
	}
}

func (a *attempt) next() bool {
	now := time.Now()
	// we always make at least one attempt.
	if a.end.IsZero() {
		a.end = now.Add(a.total)
		return true
	}

	if !now.Add(a.delay).Before(a.end) {
		return false
	}
	time.Sleep(a.delay)
	return true
}
