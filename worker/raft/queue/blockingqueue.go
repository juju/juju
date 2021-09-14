// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"context"
	"time"

	"github.com/juju/errors"
)

// Operation holds the operations that a queue can hold.
type Operation struct {
	Command []byte
	Timeout time.Duration
}

// BlockingOpQueue holds the operations in a blocking queue. The purpose of this
// queue is to allow the backing off of operations at the source of enqueueing,
// and not at the consuming of the queue.
type BlockingOpQueue struct {
	queue chan Operation
	err   chan error
}

// NewBlockingOpQueue creates a new BlockingOpQueue.
func NewBlockingOpQueue() *BlockingOpQueue {
	return &BlockingOpQueue{
		queue: make(chan Operation),
		err:   make(chan error),
	}
}

// Enqueue will add an operation to the queue. As this is a blocking queue, any
// additional enqueue operations will block and wait for subsequent operations
// to be completed.
// The design of this is to ensure that people calling this will have to
// correctly handle backing off from enqueueing.
func (q *BlockingOpQueue) Enqueue(ctx context.Context, op Operation) error {
	// Block adding the operation to the queue, but if the timeout happens
	// before then, bail out.
	select {
	case q.queue <- op:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Block on the resulting error from the queue.
	select {
	case err := <-q.err:
		return errors.Trace(err)
	case <-ctx.Done():
		return ctx.Err()
	}
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
