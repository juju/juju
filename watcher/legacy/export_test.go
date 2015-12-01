// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package legacy

import (
	"github.com/juju/juju/state/watcher"
)

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

func ExtractWorkers(workers Workers) ([]string, map[string]func() (Worker, error)) {
	return workers.ids, workers.funcs
}
