// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

var logger = loggo.GetLogger("juju.worker.uniter.container")

type containerResolver struct{}

// NewResolver returns a new resolver with determines which container related operation
// should be run based on local and remote uniter states.
func NewResolver() resolver.Resolver {
	return &containerResolver{}
}

// NextOp implements the resolver.Resolver interface.
func (r *containerResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	noOp := func() (operation.Operation, error) {
		if localState.Kind == operation.RemoteInit {
			// If we are resuming from an unexpected state, skip init.
			// Retry will occur when remotestate updates.
			return opFactory.NewSkipRemoteInit(false)
		}
		return nil, resolver.ErrNoOperation
	}
	if remoteState.ContainerRunningStatus == nil {
		return noOp()
	}
	// Check if init or workload containers are running.
	if !remoteState.ContainerRunningStatus.Initialising &&
		!remoteState.ContainerRunningStatus.Running {
		return noOp()
	}
	// If we haven't yet handled the init container.
	if !localState.OutdatedRemoteCharm && localState.ContainerRunningStatus != nil {
		if localState.ContainerRunningStatus.InitialisingTime == remoteState.ContainerRunningStatus.InitialisingTime {
			// We've already initialised the container.
			return noOp()
		}
	} else if !localState.OutdatedRemoteCharm {
		// Nothing to do
		return noOp()
	}
	switch localState.Kind {
	case operation.RunHook:
		if localState.Step == operation.Pending {
			return opFactory.NewRemoteInit(*remoteState.ContainerRunningStatus)
		}
	case operation.Continue:
		return opFactory.NewRemoteInit(*remoteState.ContainerRunningStatus)
	case operation.RemoteInit:
		if localState.Step == operation.Pending {
			return opFactory.NewRemoteInit(*remoteState.ContainerRunningStatus)
		}
		// If we are resuming from an unexpected state, skip init but retry the remote init op.
		return opFactory.NewSkipRemoteInit(true)
	}
	return noOp()
}
