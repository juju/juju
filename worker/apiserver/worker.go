// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

type CertChanger interface {
	CertChangedChan() chan params.StateServingInfo
}

type apiServerWorker struct {
	worker.Worker
	certChanged chan params.StateServingInfo
}

func (a *apiServerWorker) CertChangedChan() chan params.StateServingInfo {
	return a.certChanged
}

var NewWorker = func(
	st *state.State,
	newApiserverWorker func(st *state.State, certChanged chan params.StateServingInfo) (worker.Worker, error),
	certChanged chan params.StateServingInfo,
) (worker.Worker, error) {
	w, err := newApiserverWorker(st, certChanged)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &apiServerWorker{
		Worker:      w,
		certChanged: certChanged,
	}, nil
}
