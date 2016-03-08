// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/worker"
)

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade, err := undertaker.NewClient(apiCaller, watcher.NewNotifyWatcher)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

func NewWorker(config Config) (worker.Worker, error) {
	worker, err := NewUndertaker(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
