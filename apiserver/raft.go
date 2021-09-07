// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"time"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/queue"
)

type Logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// Queue is a blocking queue to guard access and to serialize raft applications,
// allowing for client side backoff.
type Queue interface {
	// Enqueue will add an operation to the queue. As this is a blocking queue, any
	// additional enqueue operations will block and wait for subsequent operations
	// to be completed.
	// The design of this is to ensure that people calling this will have to
	// correctly handle backing off from enqueueing.
	Enqueue(context.Context, queue.Operation) error
}

// raftMediator encapsulates raft related capabilities to the facades.
type raftMediator struct {
	queue  Queue
	logger Logger
}

// ApplyLease attempts to apply the command on to the raft FSM.
func (m *raftMediator) ApplyLease(cmd []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	err := m.queue.Enqueue(ctx, queue.Operation{
		Command: cmd,
		Timeout: timeout,
	})
	if err == nil {
		return nil
	}
	if !raft.IsNotLeaderError(err) {
		return errors.Trace(err)
	}

	// Lift the worker NotLeaderError into the apiserver NotLeaderError. Ensure
	// the correct boundaries.
	leaderErr := errors.Cause(err).(*raft.NotLeaderError)
	return apiservererrors.NewNotLeaderError(leaderErr.ServerAddress(), leaderErr.ServerID())
}
