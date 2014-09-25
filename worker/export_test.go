// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"github.com/juju/juju/state/watcher"
)

var LoadedInvalid = make(chan struct{})

func init() {
	loadedInvalid = func() {
		LoadedInvalid <- struct{}{}
	}
}

func SetEnsureErr(f func(watcher.Errer) error) {
	if f == nil {
		ensureErr = watcher.EnsureErr
	} else {
		ensureErr = f
	}
}

func EnsureErr() func(watcher.Errer) error {
	return ensureErr
}
