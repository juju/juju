// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package parallel_test

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils/parallel"
)

type result string

func (r result) Close() error {
	return nil
}

type trySuite struct{}

var _ = gc.Suite(&trySuite{})

func tryFunc(delay time.Duration, val io.Closer, err error) func(<-chan struct{}) (io.Closer, error) {
	return func(<-chan struct{}) (io.Closer, error) {
		time.Sleep(delay)
		return val, err
	}
}

func (*trySuite) TestOneSuccess(c *gc.C) {
	try := parallel.NewTry(0, nil)
	try.Start(tryFunc(0, result("hello"), nil))
	val, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, result("hello"))
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
	err = try.Start(tryFunc(0, result("goodbye"), nil))
	c.Assert(err, gc.Equals, parallel.ErrClosed)
	// Wait for the first try to deliver its result
	time.Sleep(testing.ShortWait)
	try.Kill()
	err = try.Wait()
	c.Assert(err, gc.Equals, expectErr)
}

func (*trySuite) TestOutOfOrderResults(c *gc.C) {
	try := parallel.NewTry(0, nil)
	try.Start(tryFunc(50*time.Millisecond, result("first"), nil))
	try.Start(tryFunc(10*time.Millisecond, result("second"), nil))
	r, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.Equals, result("second"))
}

func (*trySuite) TestMaxParallel(c *gc.C) {
	try := parallel.NewTry(3, nil)
	var (
		mu    sync.Mutex
		count int
		max   int
	)

	for i := 0; i < 10; i++ {
		try.Start(func(<-chan struct{}) (io.Closer, error) {
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
			return result("hello"), nil
		})
	}
	r, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.Equals, result("hello"))
	mu.Lock()
	defer mu.Unlock()
	c.Assert(max, gc.Equals, 3)
}

func (*trySuite) TestStartBlocksForMaxParallel(c *gc.C) {
	try := parallel.NewTry(3, nil)

	started := make(chan struct{})
	begin := make(chan struct{})
	go func() {
		for i := 0; i < 6; i++ {
			err := try.Start(func(<-chan struct{}) (io.Closer, error) {
				<-begin
				return nil, fmt.Errorf("an error")
			})
			started <- struct{}{}
			if i < 5 {
				c.Check(err, gc.IsNil)
			} else {
				c.Check(err, gc.Equals, parallel.ErrClosed)
			}
		}
		close(started)
	}()
	// Check we can start the first three.
	timeout := time.After(testing.LongWait)
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-timeout:
			c.Fatalf("timed out")
		}
	}
	// Check we block when going above maxParallel.
	timeout = time.After(testing.ShortWait)
	select {
	case <-started:
		c.Fatalf("Start did not block")
	case <-timeout:
	}

	// Unblock two attempts.
	begin <- struct{}{}
	begin <- struct{}{}

	// Check we can start another two.
	timeout = time.After(testing.LongWait)
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-timeout:
			c.Fatalf("timed out")
		}
	}

	// Check we block again when going above maxParallel.
	timeout = time.After(testing.ShortWait)
	select {
	case <-started:
		c.Fatalf("Start did not block")
	case <-timeout:
	}

	// Close the Try - the last request should be discarded,
	// unblocking last remaining Start request.
	try.Close()

	timeout = time.After(testing.LongWait)
	select {
	case <-started:
	case <-timeout:
		c.Fatalf("Start did not unblock after Close")
	}

	// Ensure all checks are completed
	select {
	case _, ok := <-started:
		c.Assert(ok, gc.Equals, false)
	case <-timeout:
		c.Fatalf("Start goroutine did not finish")
	}
}

func (*trySuite) TestAllConcurrent(c *gc.C) {
	try := parallel.NewTry(0, nil)
	started := make(chan chan struct{})
	for i := 0; i < 10; i++ {
		try.Start(func(<-chan struct{}) (io.Closer, error) {
			reply := make(chan struct{})
			started <- reply
			<-reply
			return result("hello"), nil
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

type gradedError int

func (e gradedError) Error() string {
	return fmt.Sprintf("error with importance %d", e)
}

func gradedErrorCombine(err0, err1 error) error {
	if err0 == nil || err0.(gradedError) < err1.(gradedError) {
		return err1
	}
	return err0
}

type multiError struct {
	errs []int
}

func (e *multiError) Error() string {
	return fmt.Sprintf("%v", e.errs)
}

func (*trySuite) TestErrorCombine(c *gc.C) {
	// Use maxParallel=1 to guarantee that all errors are processed sequentially.
	try := parallel.NewTry(1, func(err0, err1 error) error {
		if err0 == nil {
			err0 = &multiError{}
		}
		err0.(*multiError).errs = append(err0.(*multiError).errs, int(err1.(gradedError)))
		return err0
	})
	errors := []gradedError{3, 2, 4, 0, 5, 5, 3}
	for _, err := range errors {
		err := err
		try.Start(func(<-chan struct{}) (io.Closer, error) {
			return nil, err
		})
	}
	try.Close()
	val, err := try.Result()
	c.Assert(val, gc.IsNil)
	grades := err.(*multiError).errs
	sort.Ints(grades)
	c.Assert(grades, gc.DeepEquals, []int{0, 2, 3, 3, 4, 5, 5})
}

func (*trySuite) TestTriesAreStopped(c *gc.C) {
	try := parallel.NewTry(0, nil)
	stopped := make(chan struct{})
	try.Start(func(stop <-chan struct{}) (io.Closer, error) {
		<-stop
		stopped <- struct{}{}
		return nil, parallel.ErrStopped
	})
	try.Start(tryFunc(0, result("hello"), nil))
	val, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, result("hello"))

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

type closeResult struct {
	closed chan struct{}
}

func (r *closeResult) Close() error {
	close(r.closed)
	return nil
}

func (*trySuite) TestExtraResultsAreClosed(c *gc.C) {
	try := parallel.NewTry(0, nil)
	begin := make([]chan struct{}, 4)
	results := make([]*closeResult, len(begin))
	for i := range begin {
		begin[i] = make(chan struct{})
		results[i] = &closeResult{make(chan struct{})}
		i := i
		try.Start(func(<-chan struct{}) (io.Closer, error) {
			<-begin[i]
			return results[i], nil
		})
	}
	begin[0] <- struct{}{}
	val, err := try.Result()
	c.Assert(err, gc.IsNil)
	c.Assert(val, gc.Equals, results[0])

	timeout := time.After(testing.ShortWait)
	for i, r := range results[1:] {
		begin[i+1] <- struct{}{}
		select {
		case <-r.closed:
		case <-timeout:
			c.Fatalf("timed out waiting for close")
		}
	}
	select {
	case <-results[0].closed:
		c.Fatalf("result was inappropriately closed")
	case <-time.After(testing.ShortWait):
	}
}

func (*trySuite) TestEverything(c *gc.C) {
	try := parallel.NewTry(5, gradedErrorCombine)
	tries := []struct {
		startAt time.Duration
		wait    time.Duration
		val     result
		err     error
	}{{
		wait: 30 * time.Millisecond,
		err:  gradedError(3),
	}, {
		startAt: 10 * time.Millisecond,
		wait:    20 * time.Millisecond,
		val:     result("result 1"),
	}, {
		startAt: 20 * time.Millisecond,
		wait:    10 * time.Millisecond,
		val:     result("result 2"),
	}, {
		startAt: 20 * time.Millisecond,
		wait:    5 * time.Second,
		val:     "delayed result",
	}, {
		startAt: 5 * time.Millisecond,
		err:     gradedError(4),
	}}
	for _, t := range tries {
		t := t
		go func() {
			time.Sleep(t.startAt)
			try.Start(tryFunc(t.wait, t.val, t.err))
		}()
	}
	val, err := try.Result()
	if val != result("result 1") && val != result("result 2") {
		c.Errorf(`expected "result 1" or "result 2" got %#v`, val)
	}
	c.Assert(err, gc.IsNil)
}
