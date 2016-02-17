// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

func NewWorker(
	stateOpener func() (*state.State, error),
	newApiserverWorker func(st *state.State, certChanged chan params.StateServingInfo) (worker.Worker, error),
	certChanged chan params.StateServingInfo,
) (worker.Worker, error) {
	st, err := stateOpener()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newApiserverWorker(st, certChanged)
}
