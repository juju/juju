// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"fmt"
	"sync"
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/singular"
)

var _ = gc.Suite(&singularSuite{})

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type singularSuite struct {
	testing.BaseSuite
}

func (*singularSuite) TestWithMasterError(c *gc.C) {
	expectErr := fmt.Errorf("an error")
	conn := &fakeConn{
		isMasterErr: expectErr,
	}
	r, err := singular.New(newRunner(), conn)
	c.Check(err, gc.ErrorMatches, "cannot get master status: an error")
	c.Check(r, gc.IsNil)
}

func (s *singularSuite) TestWithIsMasterTrue(c *gc.C) {
	// When IsMaster returns true, workers get started on the underlying
	// runner as usual.
	s.PatchValue(&singular.PingInterval, 1*time.Millisecond)
	underlyingRunner := newRunner()
	conn := &fakeConn{
		isMaster: true,
	}
	r, err := singular.New(underlyingRunner, conn)
	c.Assert(err, gc.IsNil)

	started := make(chan struct{}, 1)
	err = r.StartWorker("worker", func() (worker.Worker, error) {
		return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
			started <- struct{}{}
			<-stop
			return nil
		}), nil
	})
	select {
	case <-started:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	err = worker.Stop(r)
	c.Assert(err, gc.IsNil)
}

var errFatal = fmt.Errorf("fatal error")

func (s *singularSuite) TestWithIsMasterFalse(c *gc.C) {
	// When IsMaster returns false, dummy workers are started that
	// do nothing except wait for the pinger to return an error.

	s.PatchValue(&singular.PingInterval, testing.ShortWait/10)

	underlyingRunner := newRunner()
	conn := &fakeConn{
		isMaster: false,
		pinged:   make(chan struct{}, 5),
	}
	r, err := singular.New(underlyingRunner, conn)
	c.Assert(err, gc.IsNil)

	err = r.StartWorker("worker", func() (worker.Worker, error) {
		c.Errorf("worker unexpectedly started")
		return nil, fmt.Errorf("no worker")
	})
	c.Assert(err, gc.IsNil)

	timeout := time.NewTimer(testing.LongWait)
	for i := 0; i < cap(conn.pinged); i++ {
		select {
		case <-conn.pinged:
		case <-timeout.C:
			c.Fatalf("timed out waiting for ping")
		}
	}
	// Cause the ping to return an error; the underlying runner
	// should then exit with the error it returned, because it's
	// fatal.
	conn.setPingErr(errFatal)
	runWithTimeout(c, "wait for underlying runner", func() {
		err = underlyingRunner.Wait()
	})
	c.Assert(err, gc.Equals, errFatal)

	// Make sure there are no more pings after the ping interval by
	// draining the channel and then making sure that at most
	// one ping arrives within the next ShortWait.
loop1:
	for {
		select {
		case <-conn.pinged:
		default:
			break loop1
		}
	}
	timeout.Reset(testing.ShortWait)
	n := 0
loop2:
	for {
		select {
		case <-conn.pinged:
			c.Assert(n, jc.LessThan, 2)
			n++
		case <-timeout.C:
			break loop2
		}
	}
}

func (s *singularSuite) TestPingCalledOnceOnlyForSeveralWorkers(c *gc.C) {
	// Patch the ping interval to a large value, start several workers
	// and check that Ping is only called once.
	s.PatchValue(&singular.PingInterval, testing.LongWait)

	underlyingRunner := newRunner()
	conn := &fakeConn{
		isMaster: false,
		pinged:   make(chan struct{}, 2),
	}

	r, err := singular.New(underlyingRunner, conn)
	c.Assert(err, gc.IsNil)

	for i := 0; i < 5; i++ {
		name := fmt.Sprint("worker", i)
		err := r.StartWorker(name, func() (worker.Worker, error) {
			c.Errorf("worker unexpectedly started")
			return nil, fmt.Errorf("no worker")
		})
		c.Assert(err, gc.IsNil)
	}
	time.Sleep(testing.ShortWait)
	n := 0
loop:
	for {
		select {
		case <-conn.pinged:
			n++
		default:
			break loop
		}
	}
	c.Assert(n, gc.Equals, 1)
}

func newRunner() worker.Runner {
	return worker.NewRunner(
		func(err error) bool {
			return err == errFatal
		},
		func(err0, err1 error) bool { return true },
	)
}

type fakeConn struct {
	isMaster    bool
	isMasterErr error

	pinged chan struct{}

	mu      sync.Mutex
	pingErr error
}

func (c *fakeConn) IsMaster() (bool, error) {
	return c.isMaster, c.isMasterErr
}

func (c *fakeConn) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case c.pinged <- struct{}{}:
	default:
	}
	return c.pingErr
}

func (c *fakeConn) setPingErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pingErr = err
}

func runWithTimeout(c *gc.C, description string, f func()) {
	done := make(chan struct{})
	go func() {
		f()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(testing.LongWait):
		c.Fatalf("time out, %s", description)
	}
}
