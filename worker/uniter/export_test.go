// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"time"

	"github.com/juju/juju/testing"
)

func SetUniterObserver(u *Uniter, observer UniterExecutionObserver) {
	u.observer = observer
}

var (
	ActiveCollectMetricsTimer = &activeCollectMetricsTimer
	ActiveSendMetricsTimer    = &activeSendMetricsTimer
	IdleWaitTime              = &idleWaitTime
	LeadershipGuarantee       = &leadershipGuarantee
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
	case <-time.After(testing.LongWait):
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
		c: make(chan time.Time),
	}
}

func NewTestingMetricsTimerChooser(collector TimedSignal, sender TimedSignal) *timerChooser {
	return &timerChooser{
		collector: collector,
		sender:    sender,
		inactive:  inactiveMetricsTimer,
	}
}

func UpdateStatusSignal(now, lastSignal time.Time, interval time.Duration) <-chan time.Time {
	return updateStatusSignal(now, lastSignal, interval)
}

func ActiveCollectMetricsSignal(now, lastSignal time.Time, interval time.Duration) <-chan time.Time {
	return activeCollectMetricsTimer(now, lastSignal, interval)
}
