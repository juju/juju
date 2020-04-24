// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"github.com/juju/worker/v2"

	jworker "github.com/juju/juju/worker"
)

func NewPeriodicWorkerForTests(api Facade, params *VersionCheckerParams) worker.Worker {
	w := &toolsVersionWorker{
		api:    api,
		params: params,
	}
	periodicCall := func(stop <-chan struct{}) error {
		return w.doCheck()
	}
	return jworker.NewPeriodicWorker(periodicCall, params.CheckInterval, jworker.NewTimer)
}
