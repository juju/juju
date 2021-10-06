// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raft/queue"
	"github.com/juju/juju/worker/raft"
)

type Logger interface {
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
	Enqueue(queue.Operation) error
}

const (
	// CoalesceAmount holds the amount to coalesce for a given apply.
	CoalesceAmount = 1
)

// raftMediator encapsulates raft related capabilities to the facades.
type raftMediator struct {
	queue  Queue
	logger Logger

	tomb       *tomb.Tomb
	operations chan operation
	clock      clock.Clock
}

func newRaftMediator(queue Queue, logger Logger, clock clock.Clock) *raftMediator {
	mediator := &raftMediator{
		queue:  queue,
		logger: logger,

		tomb:       new(tomb.Tomb),
		operations: make(chan operation),
		clock:      clock,
	}

	mediator.tomb.Go(mediator.loop)

	return mediator
}

type operation struct {
	cmd  []byte
	done func(error)
}

// ApplyLease attempts to apply the command on to the raft FSM. It only takes a
// command and enqueues that against the raft instance. If the raft instance is
// already processing a application, then back pressure is applied to the
// caller and a ErrEnqueueDeadlineExceeded will be sent. It's up to the caller
// to retry or drop depending on how the retry algorithm is implemented.
func (m *raftMediator) ApplyLease(cmd []byte) error {
	if m.logger.IsTraceEnabled() {
		m.logger.Tracef("Applying Lease with command %s", string(cmd))
	}

	done := make(chan struct{})

	var err error
	m.operations <- operation{
		cmd: cmd,
		done: func(e error) {
			defer close(done)
			err = e
		},
	}

	select {
	case <-done:
	case <-m.clock.After(time.Second * 5):
		return lease.ErrDropped
	}

	if err == nil {
		return nil
	}
	if !raft.IsNotLeaderError(err) {
		return errors.Trace(err)
	}

	// Lift the worker NotLeaderError into the apiserver NotLeaderError. Ensure
	// the correct boundaries.
	leaderErr := errors.Cause(err).(*raft.NotLeaderError)
	m.logger.Tracef("Not currently the leader, go to %v %v", leaderErr.ServerAddress(), leaderErr.ServerID())
	return apiservererrors.NewNotLeaderError(leaderErr.ServerAddress(), leaderErr.ServerID())
}

func (m *raftMediator) loop() error {
	var ops []operation
	for {
		select {
		case op := <-m.operations:
			ops = append(ops, op)
			if len(ops) >= CoalesceAmount {
				m.enqueue(ops)
				ops = make([]operation, 0)
			}

		case <-m.clock.After(time.Millisecond * 100):
			if len(ops) == 0 {
				continue
			}
			m.enqueue(ops)
			ops = make([]operation, 0)
		}
	}
}

func (m *raftMediator) enqueue(ops []operation) {
	var commands [][]byte
	for _, op := range ops {
		commands = append(commands, op.cmd)
	}

	err := m.queue.Enqueue(queue.Operation{
		Commands: commands,
	})

	var indexErr *raft.IndexError
	if e, ok := err.(*raft.IndexError); ok {
		indexErr = e
	}

	// If we get now errors whilst enqueuing on the queue, we can safely just
	// callback all the errors.
	for i, op := range ops {
		if err == nil {
			op.done(nil)
			continue
		} else if raft.IsNotLeaderError(err) {
			op.done(err)
			continue
		} else if indexErr == nil {
			op.done(err)
			continue
		}

		// Handle the fact that we've got an error for a given operation.
		if idx := indexErr.Index(); i < idx {
			op.done(nil)
		} else if i == idx {
			op.done(indexErr.RawError())
		} else {
			op.done(lease.ErrDropped)
		}
	}
}
