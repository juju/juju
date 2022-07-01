// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/v3/core/raft/queue"
	"github.com/juju/juju/v3/core/raftlease"
)

const (
	// InitialApplyTimeout is the initial timeout for applying a time. When
	// starting up a raft backend, on some machines it might take more than the
	// running apply timeout. For that reason, we allow a grace period when
	// initializing.
	InitialApplyTimeout = time.Second * 5
	// ApplyTimeout is the timeout for applying a command in an operation. It
	// is expected that raft can commit a log with in this timeout.
	ApplyTimeout = time.Second * 2
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
	// manner. This returns a future that can be used to wait on the
	// application. An optional timeout can be provided to limit the amount of
	// time we wait for the command to be started. This must be run on the
	// leader, or it will fail.
	Apply([]byte, time.Duration) raft.ApplyFuture
}

// ApplierMetrics defines an interface for recording the application of a log.
type ApplierMetrics interface {
	// Record times how long an apply operation took, along with if it failed or
	// not. This can be used to understand if we're hitting issues with the
	// underlying raft instance.
	Record(start time.Time, result string)
}

// Applier applies a new operation against a raft instance.
type Applier struct {
	raft         Raft
	notifyTarget raftlease.NotifyTarget
	metrics      ApplierMetrics
	clock        clock.Clock
	logger       Logger
}

// NewApplier creates a new Applier.
func NewApplier(
	raft Raft, target raftlease.NotifyTarget, metrics ApplierMetrics, clock clock.Clock, logger Logger,
) LeaseApplier {
	return &Applier{
		raft:         raft,
		notifyTarget: target,
		metrics:      metrics,
		clock:        clock,
		logger:       logger,
	}
}

// ApplyOperation applies a lease operation against the raft instance. If the
// raft instance isn't the leader, then an error is returned with the leader
// information if available.
// This Raft spec outlines this "The first option, which we recommend ..., is
// for the server to reject the request and return to the client the address of
// the leader, if known." (see 6.2.1).
// If the leader is the current raft instance, then attempt to apply it to
// the fsm.
func (a *Applier) ApplyOperation(ops []queue.OutOperation, applyTimeout time.Duration) {
	for i, op := range ops {
		a.applyOperation(i, op, applyTimeout)
	}
}

func (a *Applier) applyOperation(i int, op queue.OutOperation, applyTimeout time.Duration) {
	start := a.clock.Now()

	if a.logger.IsTraceEnabled() {
		a.logger.Tracef("Applying command %d %v", i, string(op.Command))
	}

	// Back pressure is applied when you attempt to apply, reading from the
	// FSM future should be read async and that's done in another goroutine.
	future := a.raft.Apply(op.Command, applyTimeout)

	// Once you've written to the log, you need to also write to the notify
	// target.
	go func() {
		var err error
		defer func() {
			result := "success"
			if err != nil {
				result = "failure"
			}
			a.metrics.Record(start, result)
		}()

		if err = future.Error(); err != nil {
			op.Done(a.handleRaftError(err))
			return
		}

		response := future.Response()
		fsmResponse, ok := response.(raftlease.FSMResponse)
		if !ok {
			a.logger.Criticalf("programming error: expected an FSMResponse, got %T: %#v", response, response)
			err = errors.Errorf("invalid FSM response")
			op.Done(err)
			return
		}
		if err = fsmResponse.Error(); err != nil {
			op.Done(err)
			return
		}

		// If the notify fails here, just ignore it and log the error,
		// so that the operator can at least see the issues when
		// inspecting the controller logs.
		if err := fsmResponse.Notify(a.notifyTarget); err != nil {
			a.logger.Errorf("failed to notify: %v", err)
		}

		op.Done(nil)
	}()
}

func (a *Applier) handleRaftError(raftError error) error {
	a.logger.Errorf("Raft future error: %v", raftError)

	switch raftError {
	case raft.ErrNotLeader, raft.ErrLeadershipLost, raft.ErrLeadershipTransferInProgress:
		if err := a.currentLeaderState(); err != nil {
			return err
		}
	}

	return raftError
}

func (a *Applier) currentLeaderState() error {
	if state := a.raft.State(); state != raft.Leader {
		leaderAddress := a.raft.Leader()

		a.logger.Infof("Attempt to apply the lease failed, we're not the leader. State: %v, Leader: %v", state, leaderAddress)

		// If the leaderAddress is empty, this implies that either we don't
		// have a leader or there is no raft cluster setup.
		if leaderAddress == "" {
			// Return an error indicating we don't have a leader.
			// It is up to the client to work out what to do next.
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
	return nil
}
