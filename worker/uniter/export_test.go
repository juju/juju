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

func (c *HookContext) ActionResultsMap() map[string]interface{} {
	if c.actionData == nil {
		panic("context not running an action")
	}
	return c.actionData.ResultsMap
}

func (c *HookContext) ActionFailed() bool {
	if c.actionData == nil {
		panic("context not running an action")
	}
	return c.actionData.ActionFailed
}

func (c *HookContext) ActionMessage() string {
	if c.actionData == nil {
		panic("context not running an action")
	}
	return c.actionData.ResultsMessage
}

func GetStubActionContext(in map[string]interface{}) *HookContext {
	return &HookContext{
		actionData: &actionData{
			ResultsMap: in,
		},
	}
}

// PatchMeterStatus changes the meter status of the context.
func (ctx *HookContext) PatchMeterStatus(code, info string) func() {
	oldMeterStatus := ctx.meterStatus
	ctx.meterStatus = &meterStatus{
		code: code,
		info: info,
	}
	return func() {
		ctx.meterStatus = oldMeterStatus
	}
}

var (
	MergeEnvironment    = mergeEnvironment
	SearchHook          = searchHook
	HookCommand         = hookCommand
	LookPath            = lookPath
	ValidatePortRange   = validatePortRange
	TryOpenPorts        = tryOpenPorts
	TryClosePorts       = tryClosePorts
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
