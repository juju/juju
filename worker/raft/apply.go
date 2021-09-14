// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/worker/raft/queue"
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

// ApplyOperation applies an lease opeartion against the raft instance. If the
// raft instance isn't the leader, then an error is returned with the leader
// information if available.
// This Raft spec outlines this "The first option, which we recommend ..., is
// for the server to reject the request and return to the client the address of
// the leader, if known." (see 6.2.1).
// If the leader is the current raft instance, then attempt to apply it to
// the fsm.
func ApplyOperation(r Raft, op queue.Operation, notifyTarget raftlease.NotifyTarget, logger Logger) error {
	if state := r.State(); state != raft.Leader {
		leaderAddress := r.Leader()

		logger.Debugf("Attempt to apply the lease failed, we're not the leader. State: %v, Leader: %v", state, leaderAddress)

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
		future := r.GetConfiguration()
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

	logger.Tracef("Applying command %v", string(op.Command))

	future := r.Apply(op.Command, op.Timeout)
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

	fsmResponse.Notify(notifyTarget)

	return nil
}
