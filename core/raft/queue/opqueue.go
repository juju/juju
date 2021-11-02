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
	ErrDeadlineExceeded = deadlineExceededErr("enqueueing deadline exceeded")

	// ErrCanceled is a sentinel error for when the operation has been stopped.
	ErrCanceled = canceledErr("enqueueing canceled")
)

type deadlineExceededErr string

func (e deadlineExceededErr) Error() string {
	return string(e)
}

// IsDeadlineExceeded checks to see if the underlying error is a deadline
// exceeded error.
func IsDeadlineExceeded(err error) bool {
	_, ok := errors.Cause(err).(deadlineExceededErr)
	return ok
}

type canceledErr string

func (e canceledErr) Error() string {
	return string(e)
}

// IsCanceled checks to see if the underlying error is a canceled error.
func IsCanceled(err error) bool {
	_, ok := errors.Cause(err).(canceledErr)
	return ok
}

// Operation holds the operations that a queue can hold.
type Operation struct {
	Command []byte
	Done    func(error)
	Stop    <-chan struct{}
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
		in:           make(chan ringOp),
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
	case <-q.tomb.Dying():
		op.Done(tomb.ErrDying)
	case <-q.tomb.Dead():
		op.Done(q.tomb.Err())
	case q.in <- makeRingOp(op, q.clock.Now()):
	}
}

// Queue returns the queue of operations. Removing an item from the channel
// will unblock to allow another to take it's place.
func (q *OpQueue) Queue() <-chan []Operation {
	return q.out
}

// Kill puts the tomb in a dying state for the given reason,
// closes the Dying channel, and sets Alive to false.
func (q *OpQueue) Kill(reason error) {
	q.tomb.Kill(reason)
}

// Wait blocks until all goroutines have finished running, and
// then returns the reason for their death.
func (q *OpQueue) Wait() error {
	return q.tomb.Wait()
}

const (
	minDelay   = time.Millisecond * 2
	maxDelay   = time.Millisecond * 200
	deltaDelay = maxDelay - minDelay
)

func (q *OpQueue) loop() error {
	// Ensure we close the queue channels when we've been killed.
	defer close(q.in)
	defer close(q.out)

	// Start of the loop with the a average delay timeout.
	delay := maxDelay / 2

	for {
		select {
		case <-q.tomb.Dying():
			return tomb.ErrDying
		case <-q.tomb.Dead():
			return q.tomb.Err()
		default:
		}

		ops := q.consume(delay)
		if len(ops) == 0 {
			continue
		}

		// Send the batch operations.
		select {
		case q.out <- ops:
		case <-q.tomb.Dead():
			return q.tomb.Err()
		}

		delay = q.calculateDelay(delay, len(ops))
	}
}

func (q *OpQueue) calculateDelay(delay time.Duration, n int) time.Duration {
	// If the number of operations are being consumed at a quicker rate
	// (larger slice), then the delay should be reduced between timeouts, so
	// we could process more messages during a stampeding herd.
	// During calmer times, the delay will slowly move to the maxDelay to
	// prevent the loop running too tightly.
	percentage := float64(n) / float64(q.maxBatchSize)
	fixed := deltaDelay - time.Duration(float64(deltaDelay)*percentage)
	delay += (fixed - delay) / 2

	// Round the delay, before doing the min and max checks.
	delay = delay.Round(time.Millisecond)
	if delay <= minDelay {
		delay = minDelay
	} else if delay >= maxDelay {
		delay = maxDelay
	}

	return delay
}

func (q *OpQueue) consume(delay time.Duration) []Operation {
	ops := make([]Operation, 0)
	timeout := q.clock.After(delay)

	for {
		select {
		case op := <-q.in:
			if isCanceled(op) {
				break
			}

			// Ensure we don't put operations that have already expired.
			if q.clock.Now().After(op.ExpiredTime) {
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

func isCanceled(op ringOp) bool {
	select {
	case <-op.Operation.Stop:
		op.Operation.Done(ErrCanceled)
		return true
	default:
		return false
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
