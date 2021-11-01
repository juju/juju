// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
)

const (
	// applyTimeout is the maximum time we will wait
	// for Raft to apply the clock advance operation.
	applyTimeout = 5 * time.Second

	// leaderTimeout is the maximum time we spend retrying the application of a
	// clock advance operation that is failing because we are not the leader.
	leaderTimeout = 10 * time.Second
)

// RaftApplier describes methods required to coordinate
// applying commands to a Raft leader node.
type RaftApplier interface {
	// Apply applies the input command to Raft's FSM and returns
	// a future that can be used to wait for completion.
	// A non-zero timeout limits the time that we will wait for
	// the operation to be started.
	// Attempting to apply commands to a non-leader node causes
	// an error to be returned.
	Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture

	// State returns the current Raft node state; leader, follower etc.
	State() raft.RaftState
}

// Sleeper describes functionality for sleeping on the current Goroutine.
type Sleeper interface {
	Sleep(time.Duration)
}

// Timer describes the ability to receive a notification via
// channel after some duration has expired.
type Timer interface {
	After(time.Duration) <-chan time.Time
}

// updater implements globalClock.Updater by applying a clock advance operation
// to its raft member.
type updater struct {
	raft               RaftApplier
	clock              raftlease.ReadOnlyClock
	logger             Logger
	sleeper            Sleeper
	timer              Timer
	expiryNotifyTarget raftlease.NotifyTarget
}

func newUpdater(
	r RaftApplier,
	notifyTarget raftlease.NotifyTarget,
	clock raftlease.ReadOnlyClock,
	sleeper Sleeper,
	timer Timer,
	logger Logger,
) *updater {
	return &updater{
		raft:               r,
		expiryNotifyTarget: notifyTarget,
		clock:              clock,
		sleeper:            sleeper,
		timer:              timer,
		logger:             logger,
	}
}

// Advance applies the clock advance operation to Raft if it is the current
// leader.
func (u *updater) Advance(duration time.Duration, stop <-chan struct{}) error {
	becomingLeaderTimeout := u.timer.After(leaderTimeout)
	for {
		oldTime := u.clock.GlobalTime()
		cmd, err := u.createCommand(oldTime, oldTime.Add(duration))
		if err != nil {
			return errors.Trace(err)
		}

		future := u.raft.Apply(cmd, applyTimeout)
		if err = future.Error(); err == nil {
			raw := future.Response()
			response, ok := raw.(raftlease.FSMResponse)
			if !ok {
				return errors.Errorf("expected FSMResponse, got %T: %#v", raw, raw)
			}

			// Ensure we also check the FSMResponse error.
			if err = response.Error(); err == nil {
				response.Notify(u.expiryNotifyTarget)
				break
			}
		}

		u.logger.Warningf(err.Error())

		switch errors.Cause(err) {
		case raft.ErrNotLeader, raft.ErrLeadershipLost:
			// This updater is recruited by a worker that depends on being run
			// on the same machine/container as the leader.
			// We have some tolerance, but generally expect that a Raft
			// leadership change will cause a shutdown of the worker running
			// this updater, indicated by a message on the stop channel.
			u.logger.Warningf("clock update still pending; local Raft state is %q", u.raft.State())

		case raft.ErrEnqueueTimeout:
			// Convert the error to one retryable by the updater worker.
			return globalclock.ErrTimeout

		default:
			return errors.Trace(err)
		}

		select {
		case <-stop:
			return errors.Annotatef(lease.ErrAborted, raftlease.OperationSetTime)
		case <-becomingLeaderTimeout:
			return errors.Errorf("timed out waiting for local Raft state to be %q", raft.Leader)
		default:
			u.sleeper.Sleep(time.Second)
		}
	}

	return nil
}

func (u *updater) createCommand(oldTime, newTime time.Time) ([]byte, error) {
	cmd, err := (&raftlease.Command{
		Version:   raftlease.CommandVersion,
		Operation: raftlease.OperationSetTime,
		OldTime:   oldTime,
		NewTime:   newTime,
	}).Marshal()

	return cmd, errors.Trace(err)
}
