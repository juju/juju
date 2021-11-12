// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/raft/queue"
	"github.com/juju/juju/worker/raft"
)

const (
	// ApplyLeaseTimeout is the maximum upper bound of waiting for applying a
	// command to the lease. This should never trigger, as the underlying system
	// has other underlying shorter timeouts that should never grow to this
	// timeout. This stop gap is to ensure that we never block leases.
	ApplyLeaseTimeout = time.Second * 10
)

type Logger interface {
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
	IsTraceEnabled() bool
}

// Queue is a blocking queue to guard access and to serialize raft applications,
// allowing for client side backoff.
type Queue interface {
	// Enqueue will add an operation to the queue. As this is a blocking queue, any
	// additional enqueue operations will block and wait for subsequent operations
	// to be completed.
	// The design of this is to ensure that people calling this will have to
	// correctly handle backing off from enqueueing.
	Enqueue(queue.Operation)
}

// raftMediator encapsulates raft related capabilities to the facades.
type raftMediator struct {
	queue  Queue
	logger Logger
	clock  clock.Clock
}

// ApplyLease attempts to apply the command on to the raft FSM. It only takes a
// command and enqueues that against the raft instance. If the raft instance is
// already processing a application, then back pressure is applied to the
// caller and a ErrEnqueueDeadlineExceeded will be sent. It's up to the caller
// to retry or drop depending on how the retry algorithm is implemented.
func (m *raftMediator) ApplyLease(ctx context.Context, leaseType string, cmd []byte) error {
	if m.logger.IsTraceEnabled() {
		m.logger.Tracef("Applying %q Lease with command %s", leaseType, string(cmd))
	}

	done := make(chan error, 1)

	start := m.clock.Now()

	m.queue.Enqueue(queue.Operation{
		Type:    leaseType,
		Command: cmd,
		Stop:    ctx.Done,
		Done: func(err error) {
			// We can do this, because the caller of done, is in another
			// goroutine, otherwise this is a sure fire way to deadlock.
			if err != nil {
				done <- err
			}
			close(done)
		},
	})

	var err error
	select {
	case err = <-done:
		if m.logger.IsTraceEnabled() {
			elapsed := m.clock.Now().Sub(start)
			m.logger.Tracef("Applying lease %s took: %v with error: %s", string(cmd), elapsed, err)
		}

	case <-m.clock.After(ApplyLeaseTimeout):
		m.logger.Errorf("Applying Lease timed out, waiting for enqueue: %v", m.clock.Now().Sub(start))
		return apiservererrors.NewDeadlineExceededError("Apply lease upper bound timeout")
	}

	switch {
	case err == nil:
		return nil

	case raft.IsNotLeaderError(err):
		// Lift the worker NotLeaderError into the apiserver NotLeaderError. Ensure
		// the correct boundaries.
		leaderErr := errors.Cause(err).(*raft.NotLeaderError)
		m.logger.Tracef("Not currently the leader, go to %v %v", leaderErr.ServerAddress(), leaderErr.ServerID())
		return apiservererrors.NewNotLeaderError(leaderErr.ServerAddress(), leaderErr.ServerID())

	case queue.IsDeadlineExceeded(err):
		// If the deadline is exceeded, get original caller to handle the
		// timeout correctly.
		return apiservererrors.NewDeadlineExceededError(err.Error())

	case queue.IsCanceled(err):
		// If the apply lease is canceled from the context (facade), then let
		// the original caller handle it correctly.
		return apiservererrors.NewDeadlineExceededError(err.Error())

	}
	return errors.Trace(err)
}
