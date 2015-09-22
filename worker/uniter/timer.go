// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"math/rand"
	"time"
)

const (
	// interval at which the unit's status should be polled
	statusPollInterval = 5 * time.Minute
)

// BackoffTimer implements a timer that will signal after a
// internally stored duration. The steps as well as min and max
// durations are declared upon initialization
type BackoffTimer interface {
	// Channel returns the channel that can be listened for events
	Channel() <-chan struct{}

	// Reset will reset the timer to it's minimum value
	Reset()

	// Signal will return after a internally stored duration
	// and increase the duration for the next call
	Signal()
}

// NewBackoffTimer creates and initializer a new BackoffTimer
func NewBackoffTimer(min, max time.Duration, jitter bool, factor int64) BackoffTimer {
	return &backoffTimer{
		min:             min,
		max:             max,
		jitter:          jitter,
		factor:          factor,
		channel:         make(chan struct{}, 1),
		currentDuration: min,
	}
}

type backoffTimer struct {
	timer *time.Timer

	min    time.Duration
	max    time.Duration
	jitter bool
	factor int64

	channel chan struct{}

	currentDuration time.Duration
}

func (t *backoffTimer) Channel() <-chan struct{} {
	return t.channel
}

func (t *backoffTimer) Signal() {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(t.currentDuration, func() {
		t.channel <- struct{}{}
	})
	// Since it's a backoff timer we will increase
	// the duration after each signal.
	t.increaseDuration()
}

func (t *backoffTimer) increaseDuration() {
	current := int64(t.currentDuration)
	nextDuration := time.Duration(current * t.factor)
	if t.jitter {
		// Get a factor in [-1; 1]
		randFactor := (rand.Float64() * 2) - 1
		jitter := float64(nextDuration) * randFactor * 0.03
		nextDuration = nextDuration + time.Duration(jitter)
	}
	if nextDuration > t.max {
		nextDuration = t.max
	}
	t.currentDuration = nextDuration
}

func (t *backoffTimer) Reset() {
	if t.currentDuration > t.min {
		t.timer.Stop()
		t.currentDuration = t.min
	}
}

// updateStatusSignal returns a time channel that fires after a given interval.
func updateStatusSignal() <-chan time.Time {
	return time.After(statusPollInterval)
}

// NewUpdateStatusTimer returns a timed signal suitable for update-status hook.
func NewUpdateStatusTimer() func() <-chan time.Time {
	return updateStatusSignal
}
