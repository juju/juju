// Copyright 2013 Canonical Ltd.
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

var MergeEnvironment = mergeEnvironment

var SearchHook = searchHook

var HookCommand = hookCommand

var LookPath = lookPath
