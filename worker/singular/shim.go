// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/singular"
)

// NewFacade creates a Facade from an APICaller and a controller. It's a
// suitable default value for ManifoldConfig.NewFacade.
func NewFacade(apiCaller base.APICaller, controllerTag names.MachineTag) (Facade, error) {
	facade, err := singular.NewAPI(apiCaller, controllerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

// NewWorker calls NewFlagWorker but returns a more convenient type. It's
// a suitable default value for ManifoldConfig.NewWorker.
func NewWorker(config FlagConfig) (worker.Worker, error) {
	worker, err := NewFlagWorker(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
