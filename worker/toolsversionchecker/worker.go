// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.networktoolsversionchecker")

type EnvironmentCapable interface {
	Environment() (*state.Environment, error)
}

// VersionCheckerParams holds params for the version checker worker..
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

	f := func(stop <-chan struct{}) error {
		return w.doCheck()
	}
	return worker.NewPeriodicWorker(f, params.CheckInterval)
}

type toolsVersionWorker struct {
	st               EnvironmentCapable
	params           *VersionCheckerParams
	findTools        toolsFinder
	envVersionUpdate envVersionUpdater
}

func (w *toolsVersionWorker) doCheck() error {
	err := updateToolsAvailability(w.st, w.findTools, w.envVersionUpdate)
	if err != nil {
		return errors.Annotate(err, "cannot fetch new tools information")
	}
	return nil
}
