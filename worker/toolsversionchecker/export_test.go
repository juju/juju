// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"github.com/juju/juju/worker"
)

func NewPeriodicWorkerForTests(st EnvironmentCapable, params *VersionCheckerParams, f toolsFinder, e envVersionUpdater) worker.Worker {
	w := &toolsVersionWorker{
		st:               st,
		params:           params,
		findTools:        f,
		envVersionUpdate: e,
	}
	periodicCall := func(stop <-chan struct{}) error {
		return w.doCheck()
	}
	return worker.NewPeriodicWorker(periodicCall, params.CheckInterval)
}

var (
	NewEnvirons = &newEnvirons
	EnvConfig   = &envConfig
)
