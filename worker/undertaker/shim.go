// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/api/watcher"
)

// NewFacade creates a Facade from a base.APICaller, by calling the
// constructor in api/undertaker that returns a more specific type.
func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade, err := undertaker.NewClient(apiCaller, watcher.NewNotifyWatcher)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

// NewFacade creates a worker.Worker from a Config, by calling the
// local constructor that returns a more specific type.
func NewWorker(config Config) (worker.Worker, error) {
	worker, err := NewUndertaker(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
