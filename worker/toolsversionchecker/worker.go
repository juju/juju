// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"

	jworker "github.com/juju/juju/worker"
)

// VersionCheckerParams holds params for the version checker worker..
type VersionCheckerParams struct {
	CheckInterval time.Duration
}

type Facade interface {
	UpdateToolsVersion() error
}

// New returns a worker that periodically wakes up to try to find out and
// record the latest version of the tools so the update possibility can be
// displayed to the users on status.
var New = func(api Facade, params *VersionCheckerParams) worker.Worker {
	w := &toolsVersionWorker{
		api:    api,
		params: params,
	}

	f := func(stop <-chan struct{}) error {
		return w.doCheck()
	}
	return jworker.NewPeriodicWorker(f, params.CheckInterval, jworker.NewTimer)
}

type toolsVersionWorker struct {
	api    Facade
	params *VersionCheckerParams
}

func (w *toolsVersionWorker) doCheck() error {
	err := w.api.UpdateToolsVersion()
	return errors.Annotate(err, "cannot update agent binaries information")
}
