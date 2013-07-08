// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"launchpad.net/juju-core/state/watcher"
)

var LoadedInvalid = make(chan struct{})

func init() {
	loadedInvalid = func() {
		LoadedInvalid <- struct{}{}
	}
}

func (nw *notifyWorker) SetClosedHandler(f func(watcher.Errer) error) func(watcher.Errer) error {
	old := nw.closedHandler
	nw.closedHandler = f
	return old
}
