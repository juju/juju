// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"
)

const (
	// EnqueueTimeout is the timeout for enqueueing an operation. If an
	// operation can't be processed in the time, a ErrDeadlineExceeded is
	// returned.
	EnqueueTimeout time.Duration = time.Second * 3

	// EnqueueBatchSize dictates how many operations can be processed at any
	// one time.
	EnqueueBatchSize = 64
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
	in           chan ringOp
	out          chan []Operation
	clock        clock.Clock
	maxBatchSize int
	tomb         *tomb.Tomb
}

// NewOpQueue creates a new OpQueue.
func NewOpQueue(clock clock.Clock) *OpQueue {
	queue := &OpQueue{
		in:           make(chan ringOp, EnqueueBatchSize),
		out:          make(chan []Operation),
		clock:        clock,
		maxBatchSize: EnqueueBatchSize,
		tomb:         new(tomb.Tomb),
	}
	queue.tomb.Go(queue.loop)

	return queue
}

// Enqueue will add an operation to the queue. As this is a blocking queue, any
// additional enqueue operations will block and wait for subsequent operations
// to be completed.
// The design of this is to ensure that people calling this will have to
// correctly handle backing off from enqueueing.
func (q *OpQueue) Enqueue(op Operation) {
	select {
	case q.in <- makeRingOp(op, q.clock.Now()):
	case <-q.clock.After(EnqueueTimeout):
		op.Done(ErrDeadlineExceeded)
	}
}

// Queue returns the queue of operations. Removing an item from the channel
// will unblock to allow another to take it's place.
func (q *OpQueue) Queue() <-chan []Operation {
	return q.out
}

const (
	minDelay = time.Millisecond
	maxDelay = time.Millisecond * 200
)

func (q *OpQueue) loop() error {
	// Start of the loop with the a average delay timeout.
	delay := maxDelay / 2

	for {
		ops := q.consume(delay)
		if len(ops) == 0 {
			continue
		}

		start := q.clock.Now()
		// Send the batch operations.
		q.out <- ops

		elapsed := q.clock.Now().Sub(start)

		// If the messages are being consumed at a quicker rate, then the
		// delay should be reduced between timeouts. If the timeout is taking
		// longer, then allow the timeout to grow.
		//
		// If delay is growing, just allow the timeout to just jump to the
		// difference, giving the consuming side more chance to breathe. For
		// delays reducing time, slowly compact the delay so we don't force
		// the consuming side to struggle.
		// As this is adaptive, we allow the consuming side to dictate a
		// settling delay.
		delta := elapsed - delay
		if delta <= 0 {
			delay += delta / 2
		} else {
			delay += delta
		}
		if delay <= minDelay {
			delay = minDelay
		} else if delay >= maxDelay {
			delay = maxDelay
		}
	}
}

func (q *OpQueue) consume(delay time.Duration) []Operation {
	ops := make([]Operation, 0)
	timeout := q.clock.After(delay)

	for {
		select {
		case op := <-q.in:
			// Ensure we don't put operations that have already expired.
			if q.clock.Now().After(op.ExpiredTime) {
				// 	// Ensure we tell the operation that it didn't make the cut when
				// 	// attempting to coalesce.
				op.Operation.Done(ErrDeadlineExceeded)
				break
			}
			ops = append(ops, op.Operation)
			if len(ops) == q.maxBatchSize {
				return ops
			}
		case <-timeout:
			return ops
		}
	}
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
