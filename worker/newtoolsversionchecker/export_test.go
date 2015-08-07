// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package newtoolsversionchecker

import (
	"github.com/juju/juju/worker"
)

func NewForTests(st EnvironmentCapable, params *VersionCheckerParams, f toolFinder, e envVersionUpdater) worker.Worker {
	w := &toolsVersionWorker{
		st:               st,
		params:           params,
		findTools:        f,
		envVersionUpdate: e,
	}
	return worker.NewPeriodicWorker(w.doCheck, params.CheckInterval)
}

var (
	NewEnvirons = &newEnvirons
	EnvConfig   = &envConfig
)
