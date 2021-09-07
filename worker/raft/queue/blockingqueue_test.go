// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BlockingOpQueueSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BlockingOpQueueSuite{})

func (s *BlockingOpQueueSuite) TestEnqueue(c *gc.C) {
	queue := NewBlockingOpQueue()

	go func() {
		for op := range queue.Queue() {
			c.Assert(op.Command, gc.DeepEquals, []byte("abc"))
			queue.Error() <- nil

			break
		}
	}()

	err := queue.Enqueue(context.TODO(), Operation{
		Command: []byte("abc"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BlockingOpQueueSuite) TestEnqueueWithError(c *gc.C) {
	queue := NewBlockingOpQueue()

	go func() {
		for op := range queue.Queue() {
			c.Assert(op.Command, gc.DeepEquals, []byte("abc"))
			queue.Error() <- errors.New("boom")

			break
		}
	}()

	err := queue.Enqueue(context.TODO(), Operation{
		Command: []byte("abc"),
	})
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *BlockingOpQueueSuite) TestEnqueueTimesout(c *gc.C) {
	queue := NewBlockingOpQueue()

	go func() {
		for op := range queue.Queue() {
			c.Assert(op.Command, gc.DeepEquals, []byte("abc"))
			time.Sleep(time.Millisecond * 500)
			queue.Error() <- nil

			break
		}
	}()

	ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond)
	defer cancel()

	err := queue.Enqueue(ctx, Operation{
		Command: []byte("abc"),
	})
	c.Assert(err, gc.ErrorMatches, `context deadline exceeded`)
}

func (s *BlockingOpQueueSuite) TestMultipleEnqueue(c *gc.C) {
	queue := NewBlockingOpQueue()

	cmd := func(i int) []byte {
		return []byte(fmt.Sprintf("abc-%d", i))
	}

	go func() {
		var count int
		for op := range queue.Queue() {
			c.Assert(op.Command, gc.DeepEquals, cmd(count))
			queue.Error() <- nil

			count++

			if count == 2 {
				break
			}
		}
	}()

	for i := 0; i < 2; i++ {
		err := queue.Enqueue(context.TODO(), Operation{
			Command: cmd(i),
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *BlockingOpQueueSuite) TestMultipleEnqueueWithErrors(c *gc.C) {
	queue := NewBlockingOpQueue()

	cmd := func(i int) []byte {
		return []byte(fmt.Sprintf("abc-%d", i))
	}

	go func() {
		var count int
		for op := range queue.Queue() {
			c.Assert(op.Command, gc.DeepEquals, cmd(count))
			queue.Error() <- nil

			count++
			if count == 1 {
				time.Sleep(time.Millisecond * 500)
				count++
			}
			if count == 3 {
				break
			}
		}
	}()

	err := queue.Enqueue(context.TODO(), Operation{
		Command: cmd(0),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Fail this one
	ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond)
	defer cancel()
	err = queue.Enqueue(ctx, Operation{
		Command: cmd(1),
	})
	c.Assert(err, gc.ErrorMatches, `context deadline exceeded`)

	err = queue.Enqueue(context.TODO(), Operation{
		Command: cmd(2),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BlockingOpQueueSuite) TestMultipleEnqueues(c *gc.C) {
	queue := NewBlockingOpQueue()

	cmd := func(i int) []byte {
		return []byte(fmt.Sprintf("abc-%d", i))
	}

	var commands []string
	go func() {
		for op := range queue.Queue() {
			commands = append(commands, string(op.Command))
			queue.Error() <- nil

			if len(commands) > 10 {
				break
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()

			err := queue.Enqueue(context.TODO(), Operation{
				Command: cmd(i),
			})
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}
	wg.Wait()

	c.Assert(commands, jc.SameContents, []string{
		"abc-0", "abc-1", "abc-2", "abc-3", "abc-4",
		"abc-5", "abc-6", "abc-7", "abc-8", "abc-9",
	})
}
