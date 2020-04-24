// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/migrationflag"
	"github.com/juju/juju/api/watcher"
)

// NewFacade creates a *migrationflag.Facade and returns it as a Facade.
func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := migrationflag.NewFacade(apiCaller, watcher.NewNotifyWatcher)
	return facade, nil
}

// NewWorker creates a *Worker and returns it as a worker.Worker.
func NewWorker(config Config) (worker.Worker, error) {
	worker, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
