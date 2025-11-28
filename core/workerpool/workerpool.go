// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workerpool

import (
	"context"
	"strings"
	"sync"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
)

// Task represents a unit of work which should be executed by the pool workers.
type Task struct {
	// Type is a short, optional description of the task which will be
	// included in emitted log messages.
	Type string

	// Process encapsulates the logic for processing a task.
	Process func() error
}

// WorkerPool implements a fixed-size worker pool for processing tasks in
// parallel. If any of the tasks fails, the pool workers will be shut down, and
// any detected errors (different tasks may fail concurrently) will be
// consolidated and reported when the pool's Close() method is invoked.
//
// New tasks can be enqueued by writing to the channel returned by the
// pool's Queue() method while the pool's Done() method returns a channel
// which can be used by callers to detect when an error occurred and the
// pool is shutting down.
type WorkerPool struct {
	logger logger.Logger

	// A channel used to signal workers that they should finish their
	// work and exit.
	shutdownTriggerCh chan struct{}

	// The worker pool can be stopped either via a call to Shutdown() or
	// directly by one of the workers when it encounters an error. A
	// sync.Once primitive ensures that the shutdown trigger channel can
	// only be closed once.
	shutdownTrigger   sync.Once
	closeErrorChannel sync.Once

	// A waitgroup for ensuring that all workers have exited. It is used
	// both when the pool is being shutdown and also as a barrier to ensure
	// that workers have exited before draining the error reporting channel.
	wg sync.WaitGroup

	// A buffered channel (cap equal to pool size) which workers monitor
	// for incoming processing tasks.
	taskQueueCh chan Task

	// A buffered channel (cap equal to pool size) which workers emit
	// any errors they encounter before requesting the pool to be shut
	// down.
	taskErrCh chan error

	// idleChans is a slice of unbuffered channel per worker. Idle uses this to
	// establish if the workerpool is idle and has no work.
	idleChans []chan chan int
}

// NewWorkerPool returns a pool with the taskuested number of workers. Callers
// must ensure to call the pool's Close() method to avoid leaking goroutines.
func NewWorkerPool(logger logger.Logger, size int) *WorkerPool {
	// Size must be at least one
	if size <= 0 {
		size = 1
	}

	wp := &WorkerPool{
		logger:            logger,
		shutdownTriggerCh: make(chan struct{}),
		taskQueueCh:       make(chan Task, size),
		taskErrCh:         make(chan error, size),
		idleChans:         make([]chan chan int, size),
	}
	for i := range size {
		wp.idleChans[i] = make(chan chan int)
	}

	wp.wg.Add(size)
	for workerID := 0; workerID < size; workerID++ {
		wp.logger.Tracef(context.TODO(), "worker %d: starting new worker pool", workerID)
		go wp.taskWorker(workerID)
	}

	return wp
}

// Size returns the number of workers in the pool.
func (wp *WorkerPool) Size() int {
	return cap(wp.taskQueueCh)
}

// Queue returns a channel for enqueueing processing tasks.
func (wp *WorkerPool) Queue() chan<- Task {
	return wp.taskQueueCh
}

// Done returns a channel which is closed if the pool has detected one or more
// errors and is shutting down. Callers must then invoke the pool's Close method
// to obtain any reported errors.
func (wp *WorkerPool) Done() <-chan struct{} {
	return wp.shutdownTriggerCh
}

// Idle returns true when the queue is empty and the workers are not working.
func (wp *WorkerPool) Idle(ctx context.Context) bool {
	respChan := make(chan int, len(wp.idleChans))
	for {
		numEmpty := 0
		for _, idleChan := range wp.idleChans {
			select {
			case idleChan <- respChan:
			case <-wp.shutdownTriggerCh:
				return false
			case <-ctx.Done():
				return false
			}
			select {
			case queueLength := <-respChan:
				if queueLength == 0 {
					numEmpty++
				}
			case <-wp.shutdownTriggerCh:
				return false
			case <-ctx.Done():
				return false
			}
		}
		if numEmpty == len(wp.idleChans) {
			// All workers are idle and don't see any queued tasks.
			return true
		}
		if numEmpty != 0 {
			// Some of the workers disagreed about the number of queued tasks,
			// try again until we get consensus.
			continue
		}
		// All workers see queued tasks, so we cannot be idle.
		return false
	}
}

// Close the pool and return any queued errors. The method signals and waits
// for all workers to exit before draining the worker error channel and
// returning a combined error (if any errors were reported) value. After a call
// to Shutdown, no further provision tasks will be accepted by the pool.
func (wp *WorkerPool) Close() error {
	wp.triggerShutdown()
	wp.wg.Wait() // wait for workers to exit and write any errors they encounter

	// Now we can safely close and drain the error channel.
	wp.closeErrorChannel.Do(func() {
		close(wp.taskErrCh)
	})

	var errList []string
	for err := range wp.taskErrCh {
		errList = append(errList, err.Error())
	}

	if len(errList) == 0 {
		return nil
	}
	return errors.New(strings.Join(errList, "\n"))
}

func (wp *WorkerPool) taskWorker(workerID int) {
	defer wp.wg.Done()
	for {
		select {
		case task := <-wp.taskQueueCh:
			wp.logger.Debugf(context.TODO(), "worker %d: processing task %q", workerID, task.Type)
			if err := task.Process(); err != nil {
				wp.logger.Errorf(context.TODO(), "worker %d: shutting down pool due to error while handling a %q task: %v", workerID, task.Type, err)

				// This is a buffered channel to allow every pool worker to report
				// a single error before it exits. Consequently, this call can never
				// block.
				wp.taskErrCh <- err
				wp.triggerShutdown()

				return // worker cannot process any further tasks.
			}
		case <-wp.shutdownTriggerCh:
			wp.logger.Tracef(context.TODO(), "worker %d: terminating as worker pool is shutting down", workerID)
			return
		case resp := <-wp.idleChans[workerID]:
			select {
			case resp <- len(wp.taskQueueCh):
			case <-wp.shutdownTriggerCh:
				continue
			}
		}
	}
}

// triggerShutdown notifies all workers to exit once they complete the tasks
// they are currently processing. Workers can only be notified once; subsequent
// calls to this method are no-ops.
func (wp *WorkerPool) triggerShutdown() {
	wp.shutdownTrigger.Do(func() {
		close(wp.shutdownTriggerCh)
	})
}
