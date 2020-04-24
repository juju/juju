// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/base"
	apihostkeyreporter "github.com/juju/juju/api/hostkeyreporter"
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
