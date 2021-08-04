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

// updater implements globalClock.Updater by applying a clock advance operation
// to its raft member.
type updater struct {
	raft               RaftApplier
	clock              raftlease.ReadOnlyClock
	prevTime           time.Time
	logger             Logger
	expiryNotifyTarget raftlease.NotifyTarget
}

func newUpdater(r *raft.Raft, notifyTarget raftlease.NotifyTarget, clock raftlease.ReadOnlyClock, logger Logger) *updater {
	return &updater{
		raft:               r,
		expiryNotifyTarget: notifyTarget,
		clock:              clock,
		prevTime:           clock.GlobalTime(),
		logger:             logger,
	}
}

// Advance applies the clock advance operation to Raft if it is the current
// leader.
func (u *updater) Advance(duration time.Duration, stop <-chan struct{}) error {
	for {
		newTime := u.prevTime.Add(duration)
		cmd, err := u.createCommand(newTime)
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

			response.Notify(u.expiryNotifyTarget)
			u.prevTime = newTime
			break
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
			// Reset our last known global time from the FSM and convert
			// the error to one retryable by the updater worker.
			u.prevTime = u.clock.GlobalTime()
			return globalclock.ErrTimeout

		default:
			return errors.Trace(err)
		}

		select {
		case <-stop:
			return errors.Annotatef(lease.ErrAborted, raftlease.OperationSetTime)
		case <-time.After(leaderTimeout):
			return errors.Errorf("timed out waiting for local Raft state to be %q", raft.Leader)
		default:
			time.Sleep(time.Second)
		}
	}

	return nil
}

func (u *updater) createCommand(newTime time.Time) ([]byte, error) {
	cmd, err := (&raftlease.Command{
		Version:   raftlease.CommandVersion,
		Operation: raftlease.OperationSetTime,
		OldTime:   u.prevTime,
		NewTime:   newTime,
	}).Marshal()

	return cmd, errors.Trace(err)
}
