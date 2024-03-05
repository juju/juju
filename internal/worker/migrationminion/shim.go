// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api/agent/migrationminion"
	"github.com/juju/juju/api/base"
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
