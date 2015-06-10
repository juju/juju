// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"time"
)

func SetUniterObserver(u *Uniter, observer UniterExecutionObserver) {
	u.observer = observer
}

var (
	IdleWaitTime        = &idleWaitTime
	LeadershipGuarantee = &leadershipGuarantee
)

// manualTicker will be used to generate collect-metrics events
// in a time-independent manner for testing.
type ManualTicker struct {
	c chan time.Time
}

// Tick sends a signal on the ticker channel.
func (t *ManualTicker) Tick() error {
	select {
	case t.c <- time.Now():
	default:
		return fmt.Errorf("ticker channel blocked")
	}
	return nil
}

// ReturnTimer can be used to replace the metrics signal generator.
func (t *ManualTicker) ReturnTimer(now, lastRun time.Time, interval time.Duration) <-chan time.Time {
	return t.c
}

func NewManualTicker() *ManualTicker {
	return &ManualTicker{
		c: make(chan time.Time, 1),
	}
}

func (u *Uniter) ReplaceEnterAbide(f simpleUniterFunc) {
	u.enterAbide = f
}

func (u *Uniter) ReplaceEnterIdle(f simpleUniterFunc) {
	u.enterIdle = f
}

func RunUpdateStatusOnce(u *Uniter) error {
	return runUpdateStatusOnce(u)
}

func (u *Uniter) SetUpdateStatusAt(t TimedSignal) {
	u.updateStatusAt = t
}

func (u *Uniter) SetActiveMetrics(t TimedSignal) {
	u.metricsTimer.active = t
}

func UpdateStatusSignal(now, lastSignal time.Time, interval time.Duration) <-chan time.Time {
	return updateStatusSignal(now, lastSignal, interval)
}

func ActiveMetricsSignal(now, lastSignal time.Time, interval time.Duration) <-chan time.Time {
	return activeMetricsTimer(now, lastSignal, interval)
}
