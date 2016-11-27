// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/worker"
)

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade, err := migrationmaster.NewClient(apiCaller, watcher.NewNotifyWatcher)
	return facade, errors.Trace(err)
}

func NewWorker(config Config) (worker.Worker, error) {
	worker, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
