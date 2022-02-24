// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/raftlease"
	"gopkg.in/tomb.v2"
)

const (
	// EnqueueDefaultTimeout is the timeout for enqueueing an operation. If an
	// operation can't be processed in the time, a ErrDeadlineExceeded is
	// returned.
	EnqueueDefaultTimeout = time.Second * 3

	// EnqueueExtendTimeout is the timeout for enqueueing an extended operation.
	// We want a different timeout for extensions as we want to reduce the
	// amount of application leadership churning. To give the operation a better
	// chance of succeeding.
	EnqueueExtendTimeout = time.Second * 5

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

// InOperation is used to identify operations that can be enqueued on a given
// queue.
type InOperation struct {
	Command raftlease.Command
	Done    func(error)
	Stop    func() <-chan struct{}
}

// OutOperation holds the operations that a queue can hold and is used for the
// consuming side.
type OutOperation struct {
	Command []byte
	Done    func(error)
	Stop    func() <-chan struct{}
}

// OpQueue holds the operations in a blocking queue. The purpose of this
// queue is to allow the backing off of operations at the source of enqueueing,
// and not at the consuming of the queue.
type OpQueue struct {
	in           chan ringOp
	out          chan []OutOperation
	clock        clock.Clock
	maxBatchSize int
	tomb         *tomb.Tomb
}

// NewOpQueue creates a new OpQueue.
func NewOpQueue(clock clock.Clock) *OpQueue {
	queue := &OpQueue{
		in:           make(chan ringOp),
		out:          make(chan []OutOperation),
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
func (q *OpQueue) Enqueue(op InOperation) error {
	bytes, err := op.Command.Marshal()
	if err != nil {
		return errors.Trace(err)
	}

	select {
	case <-q.tomb.Dying():
		op.Done(tomb.ErrDying)
	case <-q.tomb.Dead():
		op.Done(q.tomb.Err())
	case q.in <- makeRingOp(op.Command.Lease, bytes, op.Stop, op.Done, q.clock.Now()):
	}

	return nil
}

// Queue returns the queue of operations. Removing an item from the channel
// will unblock to allow another to take its place.
func (q *OpQueue) Queue() <-chan []OutOperation {
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

func (q *OpQueue) loop() error {
	defer close(q.out)

	for {
		select {
		case <-q.tomb.Dying():
			return tomb.ErrDying
		case <-q.tomb.Dead():
			return q.tomb.Err()
		default:
		}

		ops := q.consume()
		if len(ops) == 0 {
			continue
		}

		// Send the batch operations.
		select {
		case q.out <- ops:
		case <-q.tomb.Dead():
			return q.tomb.Err()
		}
	}
}

// Consume will read ops off the pending queue as long as there are
// blocked writers. We return whatever we have accrued whenever:
// - There are no blocked writers and we have some ops.
// - We reach the maximum batch size.
// - We are being killed.
// TODO (manadart 2021-11-08): Note that his batching takes effect when there
// are multiple concurrent callers to the Raft client API, but *not* for a
// batch of operations enqueued synchronously. Further work would be required
// to accommodate such a scenario.
func (q *OpQueue) consume() []OutOperation {
	ops := make([]OutOperation, 0)

	for {
		select {
		case op := <-q.in:
			if isCanceled(op) {
				break
			}

			// Never enqueue operations that have already expired.
			if q.clock.Now().After(op.ExpiredTime) {
				op.Done(ErrDeadlineExceeded)
				break
			}

			// TODO (stickupkid): We could use a sync.Pool for this OutOperation
			// and we can put the operation back in the pool once the stop is
			// done.
			ops = append(ops, OutOperation{
				Command: op.Command,
				Done:    op.Done,
				Stop:    op.Stop,
			})
			if len(ops) >= q.maxBatchSize {
				return ops
			}
		case <-q.tomb.Dying():
			return ops
		case <-q.tomb.Dead():
			return ops
		default:
			if len(ops) > 0 {
				return ops
			}

			// If there is no work to do, pause briefly
			// so that this loop is not thrashing CPU.
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func isCanceled(op ringOp) bool {
	stop := op.Stop
	if stop == nil {
		return false
	}
	select {
	case <-stop():
		op.Done(ErrCanceled)
		return true
	default:
		return false
	}
}

type ringOp struct {
	Command     []byte
	Stop        func() <-chan struct{}
	Done        func(error)
	ExpiredTime time.Time
}

func makeRingOp(leaseType string, cmd []byte, stop func() <-chan struct{}, done func(error), now time.Time) ringOp {
	// If we locate an extension in the queue, give it a larger timeout to prevent
	// excessive lease churning.
	timeout := EnqueueDefaultTimeout
	if leaseType == "extend" {
		timeout = EnqueueExtendTimeout
	}
	return ringOp{
		Command:     cmd,
		Stop:        stop,
		Done:        done,
		ExpiredTime: now.Add(timeout),
	}
}
