// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"context"

	"github.com/juju/worker/v4"

	jworker "github.com/juju/juju/internal/worker"
)

func NewPeriodicWorkerForTests(api Facade, params *VersionCheckerParams) worker.Worker {
	w := &toolsVersionWorker{
		api:    api,
		params: params,
	}
	periodicCall := func(ctx context.Context) error {
		return w.doCheck(ctx)
	}
	return jworker.NewPeriodicWorker(periodicCall, params.CheckInterval, jworker.NewTimer)
}
