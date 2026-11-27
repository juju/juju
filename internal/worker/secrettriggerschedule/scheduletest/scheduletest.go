// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scheduletest

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

// WaitTimerArmed waits until a timer or alarm is armed on clk.
func WaitTimerArmed(c *gc.C, clk *testclock.Clock) {
	select {
	case <-clk.Alarms():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer or alarm to be armed")
	}
}

// AssertNoTimerArmed fails the test if a timer or alarm notification is queued
// on clk.
func AssertNoTimerArmed(c *gc.C, clk *testclock.Clock) {
	select {
	case <-clk.Alarms():
		c.Fatal("unexpected timer or alarm armed")
	default:
	}
}

// EarlyTimerClock wraps testclock.Clock so tests can manually fire the active
// timer or alarm early (before its scheduled deadline).
type EarlyTimerClock struct {
	*testclock.Clock
	armed chan struct{}
	mu    sync.Mutex
	timer *earlyTimer
}

const armedChannelBuffer = 10

// NewEarlyTimerClock returns an EarlyTimerClock initialized at now.
func NewEarlyTimerClock(now time.Time) *EarlyTimerClock {
	return &EarlyTimerClock{
		Clock: testclock.NewClock(now),
		armed: make(chan struct{}, armedChannelBuffer),
	}
}

// NewTimer implements clock.Clock.
func (clk *EarlyTimerClock) NewTimer(_ time.Duration) clock.Timer {
	return earlyDurationTimer{clk.newEarlyTimer()}
}

// NewAlarm implements clock.Clock.
func (clk *EarlyTimerClock) NewAlarm(_ time.Time) clock.Alarm {
	return earlyTimeAlarm{clk.newEarlyTimer()}
}

func (clk *EarlyTimerClock) newEarlyTimer() *earlyTimer {
	tr := &earlyTimer{
		clock: clk,
		ch:    make(chan time.Time, 1),
		armed: true,
	}
	clk.mu.Lock()
	clk.timer = tr
	clk.mu.Unlock()
	clk.notifyArmed()
	return tr
}

// Fire triggers the currently armed timer or alarm at now, returning true if it
// was armed.
func (clk *EarlyTimerClock) Fire(now time.Time) bool {
	clk.mu.Lock()
	tr := clk.timer
	clk.mu.Unlock()
	return tr != nil && tr.fire(now)
}

// WaitArmed blocks until the timer or alarm is armed or reset.
func (clk *EarlyTimerClock) WaitArmed(c *gc.C) {
	select {
	case <-clk.armed:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer or alarm to be armed")
	}
}

func (clk *EarlyTimerClock) notifyArmed() {
	select {
	case clk.armed <- struct{}{}:
	default:
	}
}

type earlyDurationTimer struct {
	*earlyTimer
}

func (t earlyDurationTimer) Reset(time.Duration) bool {
	return t.reset()
}

type earlyTimeAlarm struct {
	*earlyTimer
}

func (a earlyTimeAlarm) Reset(time.Time) bool {
	return a.reset()
}

type earlyTimer struct {
	mu    sync.Mutex
	clock *EarlyTimerClock
	ch    chan time.Time
	armed bool
}

func (t *earlyTimer) Chan() <-chan time.Time { return t.ch }

func (t *earlyTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	was := t.armed
	t.armed = false
	return was
}

func (t *earlyTimer) reset() bool {
	t.mu.Lock()
	was := t.armed
	t.armed = true
	t.mu.Unlock()
	t.clock.notifyArmed()
	return was
}

func (t *earlyTimer) fire(now time.Time) bool {
	t.mu.Lock()
	if !t.armed {
		t.mu.Unlock()
		return false
	}
	t.armed = false
	t.mu.Unlock()
	t.ch <- now
	return true
}
