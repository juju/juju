// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
)

const (
	// EnqueueTimeout is the timeout for enqueueing an operation. If an
	// operation can't be processed in the time, a ErrDeadlineExceeded is
	// returned.
	EnqueueTimeout time.Duration = time.Second
)

var (
	// ErrDeadlineExceeded is a sentinel error for all exceeded deadlines for
	// operations.
	ErrDeadlineExceeded = deadlineExceeded("enqueueing deadline exceeded")
)

type deadlineExceeded string

func (e deadlineExceeded) Error() string {
	return string(e)
}

// IsDeadlineExceeded checks to see if the underlying error is a deadline
// exceeded error.
func IsDeadlineExceeded(err error) bool {
	_, ok := errors.Cause(err).(deadlineExceeded)
	return ok
}

// Operation holds the operations that a queue can hold.
type Operation struct {
	Commands [][]byte
}

func (o Operation) String() string {
	var res []string
	for _, v := range o.Commands {
		res = append(res, string(v))
	}
	return strings.Join(res, ", ")
}

// BlockingOpQueue holds the operations in a blocking queue. The purpose of this
// queue is to allow the backing off of operations at the source of enqueueing,
// and not at the consuming of the queue.
type BlockingOpQueue struct {
	queue chan Operation
	err   chan error
	clock clock.Clock
}

// NewBlockingOpQueue creates a new BlockingOpQueue.
func NewBlockingOpQueue(clock clock.Clock) *BlockingOpQueue {
	return &BlockingOpQueue{
		queue: make(chan Operation),
		err:   make(chan error),
		clock: clock,
	}
}

// Enqueue will add an operation to the queue. As this is a blocking queue, any
// additional enqueue operations will block and wait for subsequent operations
// to be completed.
// The design of this is to ensure that people calling this will have to
// correctly handle backing off from enqueueing.
func (q *BlockingOpQueue) Enqueue(op Operation) error {
	// Block adding the operation to the queue, but if the timeout happens
	// before then, bail out.
	select {
	case q.queue <- op:
	case <-q.clock.After(EnqueueTimeout):
		return ErrDeadlineExceeded
	}

	// Block on the resulting error from the queue. We need to ensure that we
	// wait for the error in a bare channel read otherwise we can deadlock if
	// queue channel if nobody read the error channel.
	return errors.Trace(<-q.err)
}

// Queue returns the queue of operations. Removing an item from the channel
// will unblock to allow another to take it's place.
func (q *BlockingOpQueue) Queue() <-chan Operation {
	return q.queue
}

// Error places the resulting error for the enqueue to pick it up.
func (q *BlockingOpQueue) Error() chan<- error {
	return q.err
}
