// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongomaster_test

import (
	"fmt"
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/cmd/jujud/agent/internal/mongomaster"
	"github.com/juju/juju/testing"
	jworker "github.com/juju/juju/worker"
)

type masterSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&masterSuite{})

func (*masterSuite) TestWithMasterError(c *gc.C) {
	expectErr := fmt.Errorf("an error")
	conn := &fakeConn{
		isMasterErr: expectErr,
	}
	r, err := mongomaster.New(newRunner(), conn)
	c.Check(err, gc.ErrorMatches, "cannot get master status: an error")
	c.Check(r, gc.IsNil)
}

func (s *masterSuite) TestWithIsMasterTrue(c *gc.C) {
	// When IsMaster returns true, workers get started on the underlying
	// runner as usual.
	s.PatchValue(&mongomaster.PingInterval, 1*time.Millisecond)
	underlyingRunner := newRunner()
	conn := &fakeConn{
		isMaster: true,
	}
	r, err := mongomaster.New(underlyingRunner, conn)
	c.Assert(err, jc.ErrorIsNil)

	started := make(chan struct{}, 1)
	err = r.StartWorker("worker", func() (worker.Worker, error) {
		return jworker.NewSimpleWorker(func(stop <-chan struct{}) error {
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
	c.Assert(err, jc.ErrorIsNil)
}

var errFatal = fmt.Errorf("fatal error")

func (s *masterSuite) TestWithIsMasterFalse(c *gc.C) {
	// When IsMaster returns false, dummy workers are started that
	// do nothing except wait for the pinger to return an error.

	s.PatchValue(&mongomaster.PingInterval, testing.ShortWait/10)

	underlyingRunner := newRunner()
	conn := &fakeConn{
		isMaster: false,
		pinged:   make(chan struct{}, 5),
	}
	r, err := mongomaster.New(underlyingRunner, conn)
	c.Assert(err, jc.ErrorIsNil)

	err = r.StartWorker("worker", func() (worker.Worker, error) {
		c.Errorf("worker unexpectedly started")
		return nil, fmt.Errorf("no worker")
	})
	c.Assert(err, jc.ErrorIsNil)

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

func (s *masterSuite) TestPingCalledOnceOnlyForSeveralWorkers(c *gc.C) {
	// Patch the ping interval to a large value, start several workers
	// and check that Ping is only called once.
	s.PatchValue(&mongomaster.PingInterval, testing.LongWait)

	underlyingRunner := newRunner()
	conn := &fakeConn{
		isMaster: false,
		pinged:   make(chan struct{}, 2),
	}

	r, err := mongomaster.New(underlyingRunner, conn)
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 5; i++ {
		name := fmt.Sprint("worker", i)
		err := r.StartWorker(name, func() (worker.Worker, error) {
			c.Errorf("worker unexpectedly started")
			return nil, fmt.Errorf("no worker")
		})
		c.Assert(err, jc.ErrorIsNil)
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

func newRunner() *worker.Runner {
	return worker.NewRunner(worker.RunnerParams{
		IsFatal: func(err error) bool {
			return err == errFatal
		},
	})
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
