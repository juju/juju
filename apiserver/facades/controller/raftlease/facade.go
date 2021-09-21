// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
)

// RaftLeaseV1 informs users of the API, what is contained in Facade version 1.
type RaftLeaseV1 interface {
	ApplyLease(args params.LeaseOperations) (params.ErrorResults, error)
}

// Facade allows for modification of the underlying raft instance from a
// controller facade.
type Facade struct {
	auth facade.Authorizer
	raft facade.RaftContext
}

// NewFacade create a facade for handling raft leases.
func NewFacadeV1(context facade.Context) (*Facade, error) {
	auth := context.Auth()
	raft := context.Raft()

	return NewFacade(auth, raft)
}

// NewFacade create a Facade from just the required dependencies.
func NewFacade(auth facade.Authorizer, raft facade.RaftContext) (*Facade, error) {
	if !auth.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &Facade{
		auth: auth,
		raft: raft,
	}, nil
}

// ApplyLease is a bulk API to allow applying lease operations to a raft
// context. If the current controller is not the leader, then a NotLeaderError
// is returned. Information about where they can locate the leader maybe
// supplied in the error message, but isn't guaranteed.
// If no information is supplied, it is expected that the client performs their
// own algorithm to locate the leader (roundrobin or listen to the apidetails
// topic).
func (facade *Facade) ApplyLease(args params.LeaseOperations) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Operations))
	for k, op := range args.Operations {
		if err := facade.raft.ApplyLease([]byte(op.Command)); err != nil {
			results[k] = params.ErrorResult{
				Error: apiservererrors.ServerError(err),
			}
			break
		}
	}
	return params.ErrorResults{
		Results: results,
	}, nil
}
