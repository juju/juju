// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package raftlease implements the API for sending raft lease messages between
// api servers.
package raftlease

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

const facadeName = "RaftLease"

// API provides access to the raft lease API.
type API struct {
	facade base.FacadeCaller
	caller base.APICaller
}

// NewAPI creates a new client-side raft lease API.
func NewAPI(caller base.APICaller) *API {
	facadeCaller := base.NewFacadeCaller(caller, facadeName)
	return &API{
		facade: facadeCaller,
		caller: caller,
	}
}

// ApplyLease attempts to apply a lease against the given controller. If the
// controller is not the leader, then an error to redirect to a new leader will
// be given.
func (api *API) ApplyLease(command params.LeaseOperationCommand) error {
	var results params.ErrorResults
	err := api.facade.FacadeCall("ApplyLease", params.LeaseOperationsV2{
		Operations: []params.LeaseOperationCommand{command},
	}, &results)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}

	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return apiservererrors.RestoreError(result.Error)
	}
	return nil
}
