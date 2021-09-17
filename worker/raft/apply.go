// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/worker/raft/queue"
)

const (
	// InitialApplyTimeout is the initial timeout for applying a time. When
	// starting up a raft backend, on some machines it might take more than the
	// running apply timeout. For that reason, we allow a grace period when
	// initializing.
	InitialApplyTimeout time.Duration = time.Second * 5
	// ApplyTimeout is the timeout for applying a command in an operation. It
	// is expected that raft can commit a log with in this timeout.
	ApplyTimeout time.Duration = time.Second * 2
)

// Raft defines a local use Raft instance.
type Raft interface {
	// State is used to return the current raft state.
	State() raft.RaftState

	// Leader is used to return the current leader of the cluster.
	// It may return empty string if there is no current leader
	// or the leader is unknown.
	Leader() raft.ServerAddress

	// GetConfiguration returns the latest configuration. This may not yet be
	// committed. The main loop can access this directly.
	GetConfiguration() raft.ConfigurationFuture

	// Apply is used to apply a command to the FSM in a highly consistent
	// manner. This returns a future that can be used to wait on the application.
	// An optional timeout can be provided to limit the amount of time we wait
	// for the command to be started. This must be run on the leader or it
	// will fail.
	Apply([]byte, time.Duration) raft.ApplyFuture
}

// Applier applies a new operation against a raft instance.
type Applier struct {
	raft         Raft
	notifyTarget raftlease.NotifyTarget
	clock        clock.Clock
	logger       Logger
}

// NewApplier creates a new Applier.
func NewApplier(raft Raft, target raftlease.NotifyTarget, clock clock.Clock, logger Logger) LeaseApplier {
	return &Applier{
		raft:         raft,
		notifyTarget: target,
		clock:        clock,
		logger:       logger,
	}
}

// ApplyOperation applies an lease opeartion against the raft instance. If the
// raft instance isn't the leader, then an error is returned with the leader
// information if available.
// This Raft spec outlines this "The first option, which we recommend ..., is
// for the server to reject the request and return to the client the address of
// the leader, if known." (see 6.2.1).
// If the leader is the current raft instance, then attempt to apply it to
// the fsm.
func (a *Applier) ApplyOperation(op queue.Operation, applyTimeout time.Duration) error {
	if state := a.raft.State(); state != raft.Leader {
		leaderAddress := a.raft.Leader()

		a.logger.Infof("Attempt to apply the lease failed, we're not the leader. State: %v, Leader: %v", state, leaderAddress)

		// If the leaderAddress is empty, this implies that either we don't
		// have a leader or there is no raft cluster setup.
		if leaderAddress == "" {
			// Return back we don't have a leader and then it's up to the client
			// to work out what to do next.
			return NewNotLeaderError("", "")
		}

		// If we have a leader address, we hope that we can use that leader
		// address to locate the associate server ID. The server ID can be used
		// as a mapping for the machine ID.
		future := a.raft.GetConfiguration()
		if err := future.Error(); err != nil {
			return errors.Trace(err)
		}

		config := future.Configuration()

		// If no leader ID is located that could imply that the leader has gone
		// away during the raft.State call and the GetConfiguration call. If
		// this is the case, return the leader address and no leader ID and get
		// the client to figure out the best approach.
		var leaderID string
		for _, server := range config.Servers {
			if server.Address == leaderAddress {
				leaderID = string(server.ID)
				break
			}
		}

		return NewNotLeaderError(string(leaderAddress), leaderID)
	}

	// TODO (stickupkid): Use the raft batch apply. For now we're only ever
	// going to apply one at a time, so we can change this at a later date.
	for i, command := range op.Commands {
		a.logger.Tracef("Applying command %d %v", i, string(command))

		future := a.raft.Apply(command, applyTimeout)
		if err := future.Error(); err != nil {
			return errors.Trace(err)
		}

		response := future.Response()
		fsmResponse, ok := response.(raftlease.FSMResponse)
		if !ok {
			// This should never happen.
			panic(errors.Errorf("programming error: expected an FSMResponse, got %T: %#v", response, response))
		}
		if err := fsmResponse.Error(); err != nil {
			return errors.Trace(err)
		}

		fsmResponse.Notify(a.notifyTarget)
	}

	return nil
}
