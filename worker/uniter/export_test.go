// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
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

var MergeEnvironment = mergeEnvironment

var SearchHook = searchHook

var HookCommand = hookCommand

var LookPath = lookPath
