// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workerpool

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

var _ = tc.Suite(&ProvisionerWorkerPoolSuite{})

type ProvisionerWorkerPoolSuite struct {
}

func (s *ProvisionerWorkerPoolSuite) TestProcessMoreTasksThanWorkers(c *tc.C) {
	doneCh := make(chan struct{}, 10)
	wp := NewWorkerPool(loggertesting.WrapCheckLog(c), 5)
	c.Assert(wp.Size(), tc.Equals, 5)

	for i := 0; i < 10; i++ {
		task := Task{
			Type: "alien invasion",
			Process: func() error {
				doneCh <- struct{}{}
				return nil
			},
		}

		select {
		case wp.Queue() <- task:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timeout waiting to enqueue task")
		}
	}

	for i := 0; i < 10; i++ {
		select {
		case <-doneCh: // task ACK'd
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timeout waiting for task %d to complete", i)
		}
	}

	// Shutdown the pool and ensure that no errors got reported.
	c.Assert(wp.Close(), jc.ErrorIsNil)
}

func (s *ProvisionerWorkerPoolSuite) TestConsolidateErrors(c *tc.C) {
	var (
		wg      sync.WaitGroup
		barrier = make(chan struct{})
		wp      = NewWorkerPool(loggertesting.WrapCheckLog(c), 3)
	)

	wg.Add(3)
	for i := 0; i < 3; i++ {
		// The even-numbered workers emit an error while the odd ones
		// do not.
		var expErr error
		var task = Task{Type: "alien abduction"}
		if i%2 == 0 {
			expErr = errors.Errorf("worker %d out of fuel error", i)
		}
		task.Process = func() error {
			// Signal that we are ready to process and wait for
			// barrier to be released.
			wg.Done()
			<-barrier

			return expErr
		}

		select {
		case wp.Queue() <- task:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timeout waiting to enqueue task")
		}
	}

	// Wait for each worker to grab a taskuest and the lift the barrier
	wg.Wait()
	close(barrier)

	// Wait for all workers to complete and exit
	err := wp.Close()
	c.Assert(err, tc.Not(tc.IsNil), tc.Commentf("expected individual worker errors to be consolidated into a single error"))

	// Errors can appear in any order so we need to sort them first
	errLines := strings.Split(err.Error(), "\n")
	c.Assert(len(errLines), tc.Equals, 2, tc.Commentf("expected 2 errors to be consolidated"))
	sort.Strings(errLines)
	c.Assert(errLines[0], tc.Equals, "worker 0 out of fuel error")
	c.Assert(errLines[1], tc.Equals, "worker 2 out of fuel error")
}
