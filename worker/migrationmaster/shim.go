// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/v2/api/base"
	"github.com/juju/juju/v2/api/controller/migrationmaster"
	"github.com/juju/juju/v2/api/watcher"
)

// NewFacade attempts to create a new facade for the migration master
func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := migrationmaster.NewClient(apiCaller, watcher.NewNotifyWatcher)
	return facade, nil
}

// NewWorker creates a new Worker from the config supplied.
func NewWorker(config Config) (worker.Worker, error) {
	worker, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
