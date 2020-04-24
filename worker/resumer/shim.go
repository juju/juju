// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/resumer"
)

// NewFacade returns a useful live implementation for
// ManifoldConfig.NewFacade.
func NewFacade(apiCaller base.APICaller) (Facade, error) {
	return resumer.NewAPI(apiCaller), nil
}

// NewWorker returns a useful live implementation for
// ManifoldConfig.NewWorker.
func NewWorker(config Config) (worker.Worker, error) {
	worker, err := NewResumer(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
