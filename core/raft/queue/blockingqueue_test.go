// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"errors"
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type OpQueueSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OpQueueSuite{})

func (s *OpQueueSuite) TestEnqueue(c *gc.C) {
	now := time.Now()
	queue := NewOpQueue(testclock.NewClock(now))

	results := consumeN(c, queue, 1)

	done := make(chan error, 1)
	queue.Enqueue(Operation{
		Command: command(),
		Done: func(e error) {
			fmt.Println("DONE")
			done <- e
		},
	})

	fmt.Println("ENQUEUE")

	var err error
	select {
	case err = <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("testing took too long")
	}
	c.Assert(err, jc.ErrorIsNil)

	var count int
	for result := range results {
		c.Assert(result, gc.DeepEquals, opName(count))
		count++
	}
	c.Assert(count, gc.Equals, 1)
}

/*
func (s *OpQueueSuite) TestEnqueueWithError(c *gc.C) {
	now := time.Now()
	queue := NewOpQueue(testclock.NewClock(now))

	results := consumeNUntilErr(c, queue, 1, errors.New("boom"))

	queue.Enqueue(Operation{
		Commands: commandsN(1),
	})
	c.Assert(err, gc.ErrorMatches, `boom`)

	var count int
	for result := range results {
		c.Assert(len(result), gc.Equals, 1)
		c.Assert(result[0], gc.DeepEquals, opName(count))
		count++
	}
	c.Assert(count, gc.Equals, 1)
}

func (s *OpQueueSuite) TestEnqueueTimesout(c *gc.C) {
	now := time.Now()
	clock := testclock.NewClock(now)
	queue := NewOpQueue(clock)

	go func() {
		c.Assert(clock.WaitAdvance(time.Second*2, testing.ShortWait, 1), jc.ErrorIsNil)
	}()

	queue.Enqueue(Operation{
		Commands: commandsN(1),
	})
	c.Assert(err, gc.ErrorMatches, `enqueueing deadline exceeded`)
	c.Assert(IsDeadlineExceeded(err), jc.IsTrue)
}

func (s *OpQueueSuite) TestMultipleEnqueue(c *gc.C) {
	now := time.Now()
	queue := NewOpQueue(testclock.NewClock(now))

	results := consumeN(c, queue, 2)

	for i := 0; i < 2; i++ {
		queue.Enqueue(Operation{
			Commands: [][]byte{opName(i)},
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	var count int
	for result := range results {
		c.Assert(len(result), gc.Equals, 1)
		c.Assert(result[0], gc.DeepEquals, opName(count))
		count++
	}
	c.Assert(count, gc.Equals, 2)
}

func (s *OpQueueSuite) TestMultipleEnqueueWithErrors(c *gc.C) {
	now := time.Now()
	clock := testclock.NewClock(now)
	queue := NewOpQueue(clock)

	results := make(chan [][]byte, 3)
	go func() {
		defer close(results)

		var count int
		for op := range queue.Queue() {
			results <- op.Commands
			queue.Error() <- nil

			count++
			switch count {
			case 1:
				time.Sleep(EnqueueTimeout * 3)
				count++
			case 3:
				return
			}
		}
	}()

	queue.Enqueue(Operation{
		Commands: [][]byte{opName(0)},
	})
	c.Assert(err, jc.ErrorIsNil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		c.Assert(clock.WaitAdvance(time.Second*2, testing.ShortWait, 2), jc.ErrorIsNil)
	}()

	// Fail this one
	err = queue.Enqueue(Operation{
		Commands: [][]byte{opName(1)},
	})
	c.Assert(err, gc.ErrorMatches, `enqueueing deadline exceeded`)

	err = queue.Enqueue(Operation{
		Commands: [][]byte{opName(2)},
	})
	c.Assert(err, jc.ErrorIsNil)

	var received []string
	for result := range results {
		c.Assert(len(result), gc.Equals, 1)
		received = append(received, string(result[0]))
	}
	c.Assert(len(received), gc.Equals, 2)
	c.Assert(received, gc.DeepEquals, []string{
		"abc-0", "abc-2",
	})

	// Ensure that we actually did advance correctly.
	wg.Wait()
}

func (s *OpQueueSuite) TestMultipleEnqueues(c *gc.C) {
	now := time.Now()
	queue := NewOpQueue(testclock.NewClock(now))

	results := consumeN(c, queue, 10)

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()

			queue.Enqueue(Operation{
				Commands: [][]byte{opName(i)},
			})
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}
	wg.Wait()

	var received []string
	for result := range results {
		c.Assert(len(result), gc.Equals, 1)
		received = append(received, string(result[0]))
	}
	c.Assert(len(received), gc.Equals, 10)
	c.Assert(received, jc.SameContents, []string{
		"abc-0", "abc-1", "abc-2", "abc-3", "abc-4",
		"abc-5", "abc-6", "abc-7", "abc-8", "abc-9",
	})
}
*/
func opName(i int) []byte {
	return []byte(fmt.Sprintf("abc-%d", i))
}

func command() []byte {
	return opName(1)
}

func consumeN(c *gc.C, queue *OpQueue, n int) <-chan []byte {
	return consumeNUntilErr(c, queue, n, nil)
}

func consumeNUntilErr(c *gc.C, queue *OpQueue, n int, err error) <-chan []byte {
	results := make(chan []byte, n)

	go func() {
		defer close(results)

		var count int
		for ops := range queue.Queue() {
			for _, op := range ops {
				select {
				case results <- op.Command:
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
