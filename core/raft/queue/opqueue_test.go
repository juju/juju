// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type OpQueueSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OpQueueSuite{})

func (s *OpQueueSuite) TestEnqueue(c *gc.C) {
	queue := NewOpQueue(clock.WallClock)

	results := consumeN(c, queue, 1)

	done := make(chan error)
	go queue.Enqueue(Operation{
		Command: command(),
		Done: func(err error) {
			done <- err
		},
	})

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

func (s *OpQueueSuite) TestEnqueueWithError(c *gc.C) {
	queue := NewOpQueue(clock.WallClock)

	results := consumeNUntilErr(c, queue, 1, errors.New("boom"))

	done := make(chan error)
	go queue.Enqueue(Operation{
		Command: command(),
		Done: func(err error) {
			done <- err
		},
	})

	var err error
	select {
	case err = <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("testing took too long")
	}
	c.Assert(err, gc.ErrorMatches, `boom`)

	var count int
	for result := range results {
		c.Assert(result, gc.DeepEquals, opName(count))
		count++
	}
	c.Assert(count, gc.Equals, 1)
}

func (s *OpQueueSuite) TestMultipleEnqueue(c *gc.C) {
	queue := NewOpQueue(clock.WallClock)

	results := consumeN(c, queue, 2)

	done := make(chan error)
	for i := 0; i < 2; i++ {
		queue.Enqueue(Operation{
			Command: opName(i),
			Done: func(err error) {
				go func() {
					done <- err
				}()
			},
		})
	}

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
	c.Assert(count, gc.Equals, 2)
}

func (s *OpQueueSuite) TestMultipleEnqueueWithErrors(c *gc.C) {
	queue := NewOpQueue(clock.WallClock)

	results := make(chan []byte, 3)
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

		go queue.Enqueue(Operation{
			Command: opName(i),
			Done: func(err error) {
				done <- err
			},
		})

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
}

func (s *OpQueueSuite) TestMultipleEnqueues(c *gc.C) {
	queue := NewOpQueue(clock.WallClock)

	results := consumeN(c, queue, 10)

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()

			done := make(chan error)
			queue.Enqueue(Operation{
				Command: opName(i),
				Done: func(err error) {
					go func() {
						done <- err
					}()
				},
			})

			var err error
			select {
			case err = <-done:
			case <-time.After(testing.LongWait):
				c.Fatal("testing took too long")
			}
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}
	wg.Wait()

	var received []string
	for result := range results {
		received = append(received, string(result))
	}
	c.Assert(len(received), gc.Equals, 10)
	c.Assert(received, jc.SameContents, []string{
		"abc-0", "abc-1", "abc-2", "abc-3", "abc-4",
		"abc-5", "abc-6", "abc-7", "abc-8", "abc-9",
	})
}

func (s *OpQueueSuite) TestCalculateByGoesDown(c *gc.C) {
	queue := &OpQueue{
		maxBatchSize: EnqueueBatchSize,
	}

	// Reducing the number of operations should be less than the previous one.
	delay := maxDelay
	for i := 0; i < 20; i++ {
		newDelay := queue.calculateDelay(delay, queue.maxBatchSize)
		c.Assert(newDelay <= delay, jc.IsTrue)
		delay = newDelay
	}
	c.Assert(delay < minDelay, jc.IsFalse)
}

func (s *OpQueueSuite) TestCalculateByGoesUp(c *gc.C) {
	queue := &OpQueue{
		maxBatchSize: EnqueueBatchSize,
	}

	// Increasing the number of operations should be more than the previous one.
	delay := minDelay
	for i := 0; i < 20; i++ {
		newDelay := queue.calculateDelay(delay, 0)
		c.Assert(newDelay >= delay, jc.IsTrue)
		delay = newDelay
	}
	c.Assert(delay > maxDelay, jc.IsFalse)
}

func (s *OpQueueSuite) TestCalculateByBrownian(c *gc.C) {
	queue := &OpQueue{
		maxBatchSize: EnqueueBatchSize,
	}

	delay := maxDelay / 2
	num := queue.maxBatchSize / 2
	for i := 0; i < 10; i++ {
		n := num - 2
		if rand.Intn(2) == 0 {
			n += 2
		}
		if n >= queue.maxBatchSize {
			n = queue.maxBatchSize
		} else if n <= 0 {
			n = 0
		}

		newDelay := queue.calculateDelay(delay, n)
		if n > num {
			c.Assert(newDelay < delay, jc.IsTrue)
		} else if n < num {
			c.Assert(newDelay > delay, jc.IsTrue)
		}
		delay = newDelay

		num = n
	}
}

func opName(i int) []byte {
	return []byte(fmt.Sprintf("abc-%d", i))
}

func command() []byte {
	return opName(0)
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
