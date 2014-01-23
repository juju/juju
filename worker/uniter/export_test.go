// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"launchpad.net/juju-core/juju/osenv"
)

func SetUniterObserver(u *Uniter, observer UniterExecutionObserver) {
	u.observer = observer
}

func SetPackageProxy(settings osenv.ProxySettings) {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	proxy = settings
}
