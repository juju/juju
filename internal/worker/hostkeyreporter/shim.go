// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"

	apihostkeyreporter "github.com/juju/juju/api/agent/hostkeyreporter"
	"github.com/juju/juju/api/base"
)

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	return apihostkeyreporter.NewFacade(apiCaller), nil
}

func NewWorker(config Config) (worker.Worker, error) {
	worker, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
