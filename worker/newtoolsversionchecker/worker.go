// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package newtoolsversionchecker

import (
	"time"

	"launchpad.net/tomb"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

type EnvironmentCapable interface {
	Environment() (*state.Environment, error)
}

// VersionCheckerParams specifies how often should juju look for new
// tool versions available.
type VersionCheckerParams struct {
	CheckInterval time.Duration
}

// New returns a worker that periodically wakes up to try to find out and
// record the latest version of the tools so the update possibility can be
// displayed to the users on status.
func New(st EnvironmentCapable, params *VersionCheckerParams) worker.Worker {
	w := &toolsVersionWorker{
		st:               st,
		params:           params,
		findTools:        findTools,
		envVersionUpdate: envVersionUpdate,
	}
	return worker.NewSimpleWorker(w.loop)
}

type toolsVersionWorker struct {
	st               EnvironmentCapable
	params           *VersionCheckerParams
	findTools        toolFinder
	envVersionUpdate envVersionUpdater
}

func (w *toolsVersionWorker) loop(stopCh <-chan struct{}) error {
	for {
		select {
		case <-stopCh:
			return tomb.ErrDying
		case <-time.After(w.params.CheckInterval):
			return updateToolsAvailability(w.st, w.findTools, w.envVersionUpdate)
		}

	}
}
