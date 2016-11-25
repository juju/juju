// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/worker"
)

func NewRemoteRelationsFacade(apiCaller base.APICaller) (RemoteRelationsFacade, error) {
	facade := remoterelations.NewClient(apiCaller)
	return facade, nil
}

func NewWorker(config Config) (worker.Worker, error) {
	w, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
