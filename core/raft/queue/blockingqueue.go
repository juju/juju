// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
)

const (
	// EnqueueTimeout is the timeout for enqueueing an operation. If an
	// operation can't be processed in the time, a ErrDeadlineExceeded is
	// returned.
	EnqueueTimeout time.Duration = time.Second * 3
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
	Command []byte
	Done    func(error)
}

// OpQueue holds the operations in a blocking queue. The purpose of this
// queue is to allow the backing off of operations at the source of enqueueing,
// and not at the consuming of the queue.
type OpQueue struct {
	mutex        sync.Mutex
	queue        chan []Operation
	ring         []ringOp
	clock        clock.Clock
	maxBatchSize int
}

// NewOpQueue creates a new OpQueue.
func NewOpQueue(clock clock.Clock) *OpQueue {
	queue := &OpQueue{
		queue:        make(chan []Operation, 1),
		ring:         make([]ringOp, 0),
		clock:        clock,
		maxBatchSize: 64,
	}

	return queue
}

// Enqueue will add an operation to the queue. As this is a blocking queue, any
// additional enqueue operations will block and wait for subsequent operations
// to be completed.
// The design of this is to ensure that people calling this will have to
// correctly handle backing off from enqueueing.
func (q *OpQueue) Enqueue(op Operation) {
	select {
	case q.queue <- q.coalesce(op):
	default:
		q.mutex.Lock()
		q.ring = append(q.ring, makeRingOp(op, q.clock.Now()))
		q.mutex.Unlock()
	}
}

// Queue returns the queue of operations. Removing an item from the channel
// will unblock to allow another to take it's place.
func (q *OpQueue) Queue() <-chan []Operation {
	return q.queue
}

func (q *OpQueue) coalesce(op Operation) []Operation {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	now := q.clock.Now()

	ops := make([]Operation, 0, q.maxBatchSize)
	for i, op := range q.ring {
		// Ensure we don't put operations that have already expired.
		if op.ExpiredTime.After(now) {
			// Ensure we tell the operation that it didn't make the cut when
			// attempting to coalesce.
			op.Operation.Done(ErrDeadlineExceeded)
			continue
		}

		if len(ops) > q.maxBatchSize {
			q.ring = q.ring[i:]
			break
		}
		ops = append(ops, op.Operation)
	}

	// When coalesceing operations we can be reasonable confident that the
	// operation passed as an argument won't have expired. This means that we
	// always have a non-empty slice.
	return append(ops, op)
}

type ringOp struct {
	Operation   Operation
	ExpiredTime time.Time
}

func makeRingOp(op Operation, now time.Time) ringOp {
	return ringOp{
		Operation:   op,
		ExpiredTime: now.Add(EnqueueTimeout),
	}
}
