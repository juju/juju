// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/rpc/params"
)

// RaftLeaseV2 informs users of the API, what is contained in Facade version 2.
type RaftLeaseV2 interface {
	ApplyLease(args params.LeaseOperations) (params.ErrorResults, error)
}

type APIv2 struct {
	*Facade
}

// Facade allows for modification of the underlying raft instance from a
// controller facade.
type Facade struct {
	auth facade.Authorizer
	raft facade.RaftContext
}

// NewFacade create a Facade from just the required dependencies.
func NewFacade(context facade.Context) (*Facade, error) {
	auth := context.Auth()
	raft := context.Raft()

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
func (a *Facade) ApplyLease(ctx context.Context, args params.LeaseOperationsV2) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Operations))

	for k, op := range args.Operations {
		err := a.raft.ApplyLease(ctx, raftlease.Command{
			Version:   op.Version,
			Operation: op.Operation,
			Namespace: op.Namespace,
			ModelUUID: op.ModelUUID,
			Lease:     op.Lease,
			Holder:    op.Holder,
			Duration:  op.Duration,
			OldTime:   op.OldTime,
			NewTime:   op.NewTime,
			PinEntity: op.PinEntity,
		})
		if err == nil {
			continue
		}

		// If we're not the leader anymore, then we don't want to apply
		// any more leases. In this instance we do want to bail out
		// early, but mark all subsequent errors as the same as this
		// error.
		if errors.HasType[*apiservererrors.NotLeaderError](err) {
			// Fill up any remaining operations with the same error.
			errResult := params.ErrorResult{
				Error: apiservererrors.ServerError(err),
			}
			for i := k; i < len(args.Operations); i++ {
				results[i] = errResult
			}
			break
		}

		// A non leader error type, we should mark this one as an error, but
		// continue on applying leases.
		results[k] = params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}
	}

	return params.ErrorResults{
		Results: results,
	}, nil
}
