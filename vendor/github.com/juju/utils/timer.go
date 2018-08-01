// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"math/rand"
	"time"

	"github.com/juju/utils/clock"
)

// Countdown implements a timer that will call a provided function.
// after a internally stored duration. The steps as well as min and max
// durations are declared upon initialization and depend on
// the particular implementation.
//
// TODO(katco): 2016-08-09: This type is deprecated: lp:1611427
type Countdown interface {
	// Reset stops the timer and resets its duration to the minimum one.
	// Start must be called to start the timer again.
	Reset()

	// Start starts the internal timer.
	// At the end of the timer, if Reset hasn't been called in the mean time
	// Func will be called and the duration is increased for the next call.
	Start()
}

// NewBackoffTimer creates and initializes a new BackoffTimer
// A backoff timer starts at min and gets multiplied by factor
// until it reaches max. Jitter determines whether a small
// randomization is added to the duration.
//
// TODO(katco): 2016-08-09: This type is deprecated: lp:1611427
func NewBackoffTimer(config BackoffTimerConfig) *BackoffTimer {
	return &BackoffTimer{
		config:          config,
		currentDuration: config.Min,
	}
}

// BackoffTimer implements Countdown.
// A backoff timer starts at min and gets multiplied by factor
// until it reaches max. Jitter determines whether a small
// randomization is added to the duration.
//
// TODO(katco): 2016-08-09: This type is deprecated: lp:1611427
type BackoffTimer struct {
	config BackoffTimerConfig

	timer           clock.Timer
	currentDuration time.Duration
}

// BackoffTimerConfig is a helper struct for backoff timer
// that encapsulates config information.
//
// TODO(katco): 2016-08-09: This type is deprecated: lp:1611427
type BackoffTimerConfig struct {
	// The minimum duration after which Func is called.
	Min time.Duration

	// The maximum duration after which Func is called.
	Max time.Duration

	// Determines whether a small randomization is applied to
	// the duration.
	Jitter bool

	// The factor by which you want the duration to increase
	// every time.
	Factor int64

	// Func is the function that will be called when the countdown reaches 0.
	Func func()

	// Clock provides the AfterFunc function used to call func.
	// It is exposed here so it's easier to mock it in tests.
	Clock clock.Clock
}

// Start implements the Timer interface.
// Any existing timer execution is stopped before
// a new one is created.
func (t *BackoffTimer) Start() {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = t.config.Clock.AfterFunc(t.currentDuration, t.config.Func)

	// Since it's a backoff timer we will increase
	// the duration after each signal.
	t.increaseDuration()
}

// Reset implements the Timer interface.
func (t *BackoffTimer) Reset() {
	if t.timer != nil {
		t.timer.Stop()
	}
	if t.currentDuration > t.config.Min {
		t.currentDuration = t.config.Min
	}
}

// increaseDuration will increase the duration based on
// the current value and the factor. If jitter is true
// it will add a 0.3% jitter to the final value.
func (t *BackoffTimer) increaseDuration() {
	current := int64(t.currentDuration)
	nextDuration := time.Duration(current * t.config.Factor)
	if t.config.Jitter {
		// Get a factor in [-1; 1].
		randFactor := (rand.Float64() * 2) - 1
		jitter := float64(nextDuration) * randFactor * 0.03
		nextDuration = nextDuration + time.Duration(jitter)
	}
	if nextDuration > t.config.Max {
		nextDuration = t.config.Max
	}
	t.currentDuration = nextDuration
}
