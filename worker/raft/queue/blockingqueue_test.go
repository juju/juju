// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package queue

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BlockingOpQueueSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BlockingOpQueueSuite{})

func (s *BlockingOpQueueSuite) TestEnqueue(c *gc.C) {
	queue := NewBlockingOpQueue(clock.WallClock)

	go func() {
		for op := range queue.Queue() {
			c.Assert(op.Command, gc.DeepEquals, []byte("abc"))
			queue.Error() <- nil

			break
		}
	}()

	err := queue.Enqueue(Operation{
		Command:  []byte("abc"),
		Deadline: clock.WallClock.Now().Add(time.Second),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BlockingOpQueueSuite) TestEnqueueWithError(c *gc.C) {
	queue := NewBlockingOpQueue(clock.WallClock)

	var (
		mutex   sync.Mutex
		results []string
	)

	go func() {
		for op := range queue.Queue() {
			mutex.Lock()
			results = append(results, string(op.Command))
			mutex.Unlock()

			queue.Error() <- errors.New("boom")
			break
		}
	}()

	err := queue.Enqueue(Operation{
		Command:  []byte("abc"),
		Deadline: clock.WallClock.Now().Add(time.Second),
	})
	c.Assert(err, gc.ErrorMatches, `boom`)

	mutex.Lock()
	defer mutex.Unlock()
	c.Assert(results, jc.DeepEquals, []string{"abc"})
}

func (s *BlockingOpQueueSuite) TestEnqueueTimesout(c *gc.C) {
	queue := NewBlockingOpQueue(clock.WallClock)

	var (
		mutex   sync.Mutex
		results []string
	)

	go func() {
		var count int
		for op := range queue.Queue() {
			mutex.Lock()
			results = append(results, string(op.Command))
			mutex.Unlock()

			queue.Error() <- nil

			count++
			switch count {
			case 1:
				time.Sleep(time.Millisecond * 500)
			case 2:
				return
			}
		}
	}()

	err := queue.Enqueue(Operation{
		Command:  []byte("abc-1"),
		Deadline: clock.WallClock.Now().Add(time.Second),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = queue.Enqueue(Operation{
		Command:  []byte("abc-2"),
		Deadline: clock.WallClock.Now().Add(time.Millisecond),
	})
	c.Assert(err, gc.ErrorMatches, `deadline exceeded`)

	mutex.Lock()
	defer mutex.Unlock()
	c.Assert(results, jc.DeepEquals, []string{"abc-1"})
}

func (s *BlockingOpQueueSuite) TestMultipleEnqueue(c *gc.C) {
	queue := NewBlockingOpQueue(clock.WallClock)

	var (
		mutex   sync.Mutex
		results []string
	)

	cmd := func(i int) []byte {
		return []byte(fmt.Sprintf("abc-%d", i))
	}

	go func() {
		var count int
		for op := range queue.Queue() {
			mutex.Lock()
			results = append(results, string(op.Command))
			mutex.Unlock()
			queue.Error() <- nil

			count++

			if count == 2 {
				break
			}
		}
	}()

	for i := 0; i < 2; i++ {
		err := queue.Enqueue(Operation{
			Command:  cmd(i),
			Deadline: clock.WallClock.Now().Add(time.Second),
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	mutex.Lock()
	defer mutex.Unlock()
	c.Assert(results, jc.DeepEquals, []string{"abc-0", "abc-1"})
}

func (s *BlockingOpQueueSuite) TestMultipleEnqueueWithErrors(c *gc.C) {
	queue := NewBlockingOpQueue(clock.WallClock)

	var (
		mutex   sync.Mutex
		results []string
	)

	cmd := func(i int) []byte {
		return []byte(fmt.Sprintf("abc-%d", i))
	}

	go func() {
		var count int
		for op := range queue.Queue() {
			mutex.Lock()
			results = append(results, string(op.Command))
			mutex.Unlock()

			queue.Error() <- nil

			count++
			switch count {
			case 1:
				time.Sleep(time.Millisecond * 500)
				count++
			case 3:
				return
			}
		}
	}()

	err := queue.Enqueue(Operation{
		Command:  cmd(0),
		Deadline: clock.WallClock.Now().Add(time.Second),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Fail this one
	err = queue.Enqueue(Operation{
		Command:  cmd(1),
		Deadline: clock.WallClock.Now().Add(time.Millisecond),
	})
	c.Assert(err, gc.ErrorMatches, `deadline exceeded`)

	err = queue.Enqueue(Operation{
		Command:  cmd(2),
		Deadline: clock.WallClock.Now().Add(time.Second),
	})
	c.Assert(err, jc.ErrorIsNil)

	mutex.Lock()
	defer mutex.Unlock()
	c.Assert(results, jc.DeepEquals, []string{"abc-0", "abc-2"})
}

func (s *BlockingOpQueueSuite) TestMultipleEnqueues(c *gc.C) {
	queue := NewBlockingOpQueue(clock.WallClock)

	var (
		mutex   sync.Mutex
		results []string
	)

	cmd := func(i int) []byte {
		return []byte(fmt.Sprintf("abc-%d", i))
	}

	go func() {
		for op := range queue.Queue() {
			mutex.Lock()
			results = append(results, string(op.Command))
			num := len(results)
			mutex.Unlock()

			queue.Error() <- nil

			if num > 10 {
				break
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()

			err := queue.Enqueue(Operation{
				Command:  cmd(i),
				Deadline: clock.WallClock.Now().Add(time.Second),
			})
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}
	wg.Wait()

	c.Assert(results, jc.SameContents, []string{
		"abc-0", "abc-1", "abc-2", "abc-3", "abc-4",
		"abc-5", "abc-6", "abc-7", "abc-8", "abc-9",
	})
}
