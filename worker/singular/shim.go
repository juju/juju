// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/singular"
	"github.com/juju/juju/worker"
)

func NewFacade(apiCaller base.APICaller, controllerTag names.MachineTag) (Facade, error) {
	facade, err := singular.NewAPI(apiCaller, controllerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

func NewWorker(config FlagConfig) (worker.Worker, error) {
	worker, err := NewFlagWorker(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
