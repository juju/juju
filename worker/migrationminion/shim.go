// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/migrationminion"
)

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := migrationminion.NewClient(apiCaller)
	return facade, nil
}

func NewWorker(config Config) (worker.Worker, error) {
	worker, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
