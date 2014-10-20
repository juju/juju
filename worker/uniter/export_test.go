// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"time"

	"github.com/juju/utils/proxy"
)

func SetUniterObserver(u *Uniter, observer UniterExecutionObserver) {
	u.observer = observer
}

func (u *Uniter) GetProxyValues() proxy.Settings {
	u.proxyMutex.Lock()
	defer u.proxyMutex.Unlock()
	return u.proxy
}

func PatchMetricsTimer(newTimer func(now, lastRun time.Time, interval time.Duration) <-chan time.Time) {
	collectMetricsAt = newTimer
}

var (
	CollectMetricsTimer = collectMetricsTimer
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
