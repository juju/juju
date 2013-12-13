// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package parallel_test

import (
	"errors"
	"fmt"
	gc "launchpad.net/gocheck"
	"sync"
	"time"

	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils/parallel"
)

type trySuite struct{}

var _ = gc.Suite(&trySuite{})

func tryFunc(delay time.Duration, val interface{}, err error) func(<-chan struct{}) (interface{}, error) {
	return func(<-chan struct{}) (interface{}, error) {
		time.Sleep(delay)
		return val, err
	}
}

func (*trySuite) TestOneSuccess(c *gc.C) {
	try := parallel.NewTry(0, nil)
	try.Start(tryFunc(0, "hello", nil))
	val, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, "hello")
}

func (*trySuite) TestOneFailure(c *gc.C) {
	try := parallel.NewTry(0, nil)
	expectErr := errors.New("foo")
	err := try.Start(tryFunc(0, nil, expectErr))
	c.Assert(err, gc.IsNil)
	select {
	case <-try.Dead():
		c.Fatalf("try died before it should")
	case <-time.After(testing.ShortWait):
	}
	try.Close()
	select {
	case <-try.Dead():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for Try to complete")
	}
	val, err := try.Result()
	c.Assert(val, gc.IsNil)
	c.Assert(err, gc.Equals, expectErr)
}

func (*trySuite) TestStartReturnsErrorAfterClose(c *gc.C) {
	try := parallel.NewTry(0, nil)
	expectErr := errors.New("foo")
	err := try.Start(tryFunc(0, nil, expectErr))
	c.Assert(err, gc.IsNil)
	try.Close()
	err = try.Start(tryFunc(0, "goodbye", nil))
	c.Assert(err, gc.Equals, parallel.ErrClosed)
	// Wait for the first try to deliver its result
	time.Sleep(testing.ShortWait)
	try.Kill()
	err = try.Wait()
	c.Assert(err, gc.Equals, expectErr)
}

func (*trySuite) TestOutOfOrderResults(c *gc.C) {
	try := parallel.NewTry(0, nil)
	try.Start(tryFunc(50*time.Millisecond, "first", nil))
	try.Start(tryFunc(10*time.Millisecond, "second", nil))
	r, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.Equals, "second")
}

func (*trySuite) TestMaxParallel(c *gc.C) {
	try := parallel.NewTry(3, nil)
	var (
		mu    sync.Mutex
		count int
		max   int
	)

	for i := 0; i < 10; i++ {
		try.Start(func(<-chan struct{}) (interface{}, error) {
			mu.Lock()
			if count++; count > max {
				max = count
			}
			c.Check(count, gc.Not(jc.GreaterThan), 3)
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			count--
			mu.Unlock()
			return "hello", nil
		})
	}
	r, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.Equals, "hello")
	mu.Lock()
	defer mu.Unlock()
	c.Assert(max, gc.Equals, 3)
}

func (*trySuite) TestAllConcurrent(c *gc.C) {
	try := parallel.NewTry(0, nil)
	started := make(chan chan struct{})
	for i := 0; i < 10; i++ {
		try.Start(func(<-chan struct{}) (interface{}, error) {
			reply := make(chan struct{})
			started <- reply
			<-reply
			return "hello", nil
		})
	}
	timeout := time.After(testing.LongWait)
	for i := 0; i < 10; i++ {
		select {
		case reply := <-started:
			reply <- struct{}{}
		case <-timeout:
			c.Fatalf("timed out")
		}
	}
}

type impError int

func (e impError) Error() string {
	return fmt.Sprintf("error with importance %d", e)
}

func impErrorCompare(err0, err1 error) bool {
	return err0.(impError) > err1.(impError)
}

func (*trySuite) TestErrorImportance(c *gc.C) {
	// Use maxParallel=1 to guarantee that all errors are processed sequentially.
	try := parallel.NewTry(1, impErrorCompare)
	errors := []impError{3, 2, 4, 0, 5, 5, 3}
	for _, err := range errors {
		err := err
		try.Start(func(<-chan struct{}) (interface{}, error) {
			return nil, err
		})
	}
	try.Close()
	val, err := try.Result()
	c.Assert(val, gc.IsNil)
	c.Assert(err, gc.Equals, impError(5))
}

func (*trySuite) TestTriesAreStopped(c *gc.C) {
	try := parallel.NewTry(0, nil)
	stopped := make(chan struct{})
	try.Start(func(stop <-chan struct{}) (interface{}, error) {
		<-stop
		stopped <- struct{}{}
		return nil, nil
	})
	try.Start(tryFunc(0, "hello", nil))
	val, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, "hello")

	select {
	case <-stopped:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for stop")
	}
}

func (*trySuite) TestCloseTwice(c *gc.C) {
	try := parallel.NewTry(0, nil)
	try.Close()
	try.Close()
	val, err := try.Result()
	c.Assert(val, gc.IsNil)
	c.Assert(err, gc.IsNil)
}

func (*trySuite) TestEverything(c *gc.C) {
	try := parallel.NewTry(5, impErrorCompare)
	tries := []struct {
		startAt time.Duration
		wait    time.Duration
		val     interface{}
		err     error
	}{{
		wait: 30 * time.Millisecond,
		err:  impError(3),
	}, {
		startAt: 10 * time.Millisecond,
		wait:    20 * time.Millisecond,
		val:     "result 1",
	}, {
		startAt: 20 * time.Millisecond,
		wait:    10 * time.Millisecond,
		val:     "result 2",
	}, {
		startAt: 20 * time.Millisecond,
		wait:    5 * time.Second,
		val:     "delayed result",
	}, {
		startAt: 5 * time.Millisecond,
		err:     impError(4),
	}}
	for _, t := range tries {
		t := t
		go func() {
			time.Sleep(t.startAt)
			try.Start(tryFunc(t.wait, t.val, t.err))
		}()
	}
	val, err := try.Result()
	if val != "result 1" && val != "result 2" {
		c.Errorf(`expected "result 1" or "result 2" got %#v`, val)
	}
	c.Assert(err, gc.IsNil)
}
