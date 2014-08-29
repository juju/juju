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
	return c.actionResults.Results
}

func (c *HookContext) ActionStatus() string {
	return string(c.actionResults.Status)
}

func (c *HookContext) ActionMessage() string {
	return c.actionResults.Message
}

var MergeEnvironment = mergeEnvironment

var SearchHook = searchHook

var HookCommand = hookCommand

var LookPath = lookPath
