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

func SetMustErr(f func(watcher.Errer) error) {
	if f == nil {
		mustErr = watcher.MustErr
	} else {
		mustErr = f
	}
}

func MustErr() func(watcher.Errer) error {
	return mustErr
}
