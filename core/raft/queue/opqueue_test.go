// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/v2/core/raftlease"
)

type OpQueueSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OpQueueSuite{})

func (s *OpQueueSuite) TestEnqueueDequeue(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	results := consumeN(c, queue, 1)

	done := make(chan error)
	go func() {
		err := queue.Enqueue(InOperation{
			Command: command(opName(0)),
			Done: func(err error) {
				done <- err
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("testing took too long")
	}
	c.Assert(err, jc.ErrorIsNil)

	var count int
	for result := range results {
		c.Assert(result, gc.DeepEquals, command(opName(count)))
		count++
	}
	c.Assert(count, gc.Equals, 1)
}

func (s *OpQueueSuite) TestEnqueueWithStopped(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	canceled := make(chan struct{}, 1)
	close(canceled)

	done := make(chan error)
	go func() {
		err := queue.Enqueue(InOperation{
			Command: command(opName(0)),
			Stop: func() <-chan struct{} {
				return canceled
			},
			Done: func(err error) {
				done <- err
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("testing took too long")
	}
	c.Assert(err, gc.ErrorMatches, `enqueueing canceled`)

}

func (s *OpQueueSuite) TestEnqueueWithError(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	results := consumeNUntilErr(c, queue, 1, errors.New("boom"))

	done := make(chan error)
	go func() {
		err := queue.Enqueue(InOperation{
			Command: command(opName(0)),
			Done: func(err error) {
				done <- err
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("testing took too long")
	}
	c.Assert(err, gc.ErrorMatches, `boom`)

	var count int
	for result := range results {
		c.Assert(result, gc.DeepEquals, command(opName(count)))
		count++
	}
	c.Assert(count, gc.Equals, 1)
}

func (s *OpQueueSuite) TestSynchronousEnqueueImmediateDispatch(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	toEnqueue := 5
	go func() {
		// Synchronous enqueues should result in multiple batches despite
		// being fewer total ops than the maximum batch size.
		for i := 0; i < toEnqueue; i++ {
			err := queue.Enqueue(InOperation{
				Command: command(opName(i)),
			})
			c.Assert(err, jc.ErrorIsNil)
		}
	}()

	var results [][]OutOperation
	var totalRead int
	for ops := range queue.Queue() {
		results = append(results, ops)

		totalRead += len(ops)
		if totalRead >= toEnqueue {
			queue.Kill(nil)
		}
	}

	err := queue.Wait()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(results) > 1, jc.IsTrue)
}

func (s *OpQueueSuite) TestConcurrentEnqueueMultiDispatch(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	toEnqueue := EnqueueBatchSize * 3
	for i := 0; i < toEnqueue; i++ {
		go func(i int) {
			err := queue.Enqueue(InOperation{
				Command: command(opName(i)),
			})
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}

	var results [][]OutOperation
	var totalRead int
	for ops := range queue.Queue() {
		results = append(results, ops)

		totalRead += len(ops)
		if totalRead >= toEnqueue {
			queue.Kill(nil)
		}
	}

	err := queue.Wait()
	c.Assert(err, jc.ErrorIsNil)

	// The exact batching that occurs is variable, but as a conservative test,
	// ensure that we had a decent factor of batching - the number of batches
	// is fewer than 1/3 of the total enqueued operations.
	c.Check(len(results) > 1, jc.IsTrue)
	c.Check(len(results) < EnqueueBatchSize, jc.IsTrue)
}

func (s *OpQueueSuite) TestMultipleEnqueueWithErrors(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	results := make(chan raftlease.Command, 3)
	go func() {
		defer close(results)

		var count int
		for ops := range queue.Queue() {
			for _, op := range ops {
				var got raftlease.Command
				mErr := yaml.Unmarshal(op.Command, &got)
				c.Assert(mErr, jc.ErrorIsNil)

				select {
				case results <- got:
				case <-time.After(testing.LongWait):
					c.Fatal("timed out setting results")
				}
				if count == 1 {
					op.Done(errors.New(`boom`))
				} else {
					op.Done(nil)
				}

				count++
			}
		}
	}()

	consume := func(i int) error {
		done := make(chan error)
		defer close(done)

		go func() {
			err := queue.Enqueue(InOperation{
				Command: command(opName(i)),
				Done: func(err error) {
					done <- err
				},
			})
			c.Assert(err, jc.ErrorIsNil)
		}()

		var err error
		select {
		case err = <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("testing took too long")
		}

		return err
	}

	consumeNilErr := func(i int) {
		err := consume(i)
		c.Assert(err, jc.ErrorIsNil)
	}

	consumeErr := func(i int, e string) {
		err := consume(i)
		c.Assert(err, gc.ErrorMatches, e)
	}

	consumeNilErr(0)
	consumeErr(1, `boom`)
	consumeNilErr(2)

	queue.Kill(nil)
	err := queue.Wait()
	c.Assert(err, jc.ErrorIsNil)

	var count int
	for result := range results {
		c.Assert(result, gc.DeepEquals, command(opName(count)))
		count++
	}
	c.Assert(count, gc.Equals, 3)
}

func (s *OpQueueSuite) TestMultipleEnqueueWithStop(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	results := make(chan raftlease.Command, 2)
	go func() {
		defer close(results)

		var count int
		for ops := range queue.Queue() {
			for _, op := range ops {
				var got raftlease.Command
				mErr := yaml.Unmarshal(op.Command, &got)
				c.Assert(mErr, jc.ErrorIsNil)

				select {
				case results <- got:
				case <-time.After(testing.LongWait):
					c.Fatal("timed out setting results")
				}

				op.Done(nil)

				count++
			}
		}
	}()

	consume := func(i int, canceled <-chan struct{}) error {
		done := make(chan error)
		defer close(done)

		go func() {
			err := queue.Enqueue(InOperation{
				Command: command(opName(i)),
				Stop: func() <-chan struct{} {
					return canceled
				},
				Done: func(err error) {
					done <- err
				},
			})
			c.Assert(err, jc.ErrorIsNil)
		}()

		var err error
		select {
		case err = <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("testing took too long")
		}

		return err
	}

	consumeNilErr := func(i int) {
		err := consume(i, nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	consumeErr := func(i int, e string) {
		canceled := make(chan struct{})
		close(canceled)

		err := consume(i, canceled)
		c.Assert(err, gc.ErrorMatches, e)
	}

	consumeNilErr(0)
	consumeErr(1, `enqueueing canceled`)
	consumeNilErr(2)

	queue.Kill(nil)
	err := queue.Wait()
	c.Assert(err, jc.ErrorIsNil)

	var count, index int
	for result := range results {
		c.Assert(result, gc.DeepEquals, command(opName(index)))
		count++
		index += 2
	}
	c.Assert(count, gc.Equals, 2)
}

func (s *OpQueueSuite) TestMultipleEnqueues(c *gc.C) {
	queue := NewOpQueue(testclock.NewClock(time.Now()))

	results := consumeN(c, queue, 10)

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()

			done := make(chan error)
			err := queue.Enqueue(InOperation{
				Command: command(opName(i)),
				Done: func(err error) {
					go func() {
						done <- err
					}()
				},
			})
			c.Assert(err, jc.ErrorIsNil)

			select {
			case err = <-done:
			case <-time.After(testing.LongWait):
				c.Fatal("testing took too long")
			}
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}
	wg.Wait()

	var received []raftlease.Command
	for result := range results {
		received = append(received, result)
	}
	c.Assert(len(received), gc.Equals, 10)
	c.Assert(received, jc.SameContents, []raftlease.Command{
		command("abc-0"),
		command("abc-1"),
		command("abc-2"),
		command("abc-3"),
		command("abc-4"),
		command("abc-5"),
		command("abc-6"),
		command("abc-7"),
		command("abc-8"),
		command("abc-9"),
	})
}

func opName(i int) string {
	return fmt.Sprintf("abc-%d", i)
}

func command(lease string) raftlease.Command {
	return raftlease.Command{
		Operation: "claim",
		Lease:     lease,
	}
}

func consumeN(c *gc.C, queue *OpQueue, n int) <-chan raftlease.Command {
	return consumeNUntilErr(c, queue, n, nil)
}

func consumeNUntilErr(c *gc.C, queue *OpQueue, n int, err error) <-chan raftlease.Command {
	results := make(chan raftlease.Command, n)

	go func() {
		defer close(results)

		var count int
		for ops := range queue.Queue() {
			for _, op := range ops {
				var got raftlease.Command
				mErr := yaml.Unmarshal(op.Command, &got)
				c.Assert(mErr, jc.ErrorIsNil)

				select {
				case results <- got:
				case <-time.After(testing.LongWait):
					c.Fatal("timed out setting results")
				}

				count++
				var e error
				if count == n {
					e = err
				}
				op.Done(e)

				if count == n {
					return
				}
			}
		}
	}()

	return results
}

type QueueErrorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&QueueErrorSuite{})

func (s *QueueErrorSuite) TestDeadlineExceeded(c *gc.C) {
	err := ErrDeadlineExceeded
	c.Assert(IsDeadlineExceeded(err), jc.IsTrue)
}

func (s *QueueErrorSuite) TestDeadlineExceededOther(c *gc.C) {
	err := errors.New("bad")
	c.Assert(IsDeadlineExceeded(err), jc.IsFalse)
}
