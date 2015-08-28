// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdir_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/charmdir"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	manifold    dependency.Manifold
	getResource dependency.GetResourceFunc
}

var _ = gc.Suite(&ManifoldSuite{})

func kill(w worker.Worker) error {
	w.Kill()
	return w.Wait()
}

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.manifold = charmdir.Manifold()
	s.getResource = dt.StubGetResource(dt.StubResources{})
}

type gate struct {
	mu    sync.Mutex
	state string
}

func (g *gate) get() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

func (g *gate) set(s string) {
	g.mu.Lock()
	g.state = s
	g.mu.Unlock()
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	worker, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer kill(worker)

	var consumer charmdir.Consumer
	err = s.manifold.Output(worker, &consumer)
	c.Check(err, jc.ErrorIsNil)

	// charmdir is not available, so function will not run
	ok, err := consumer.Run(func() error { return fmt.Errorf("nope") })
	c.Check(ok, jc.IsFalse)
	c.Check(err, jc.ErrorIsNil)

	var locker charmdir.Locker
	err = s.manifold.Output(worker, &locker)
	c.Check(err, jc.ErrorIsNil)
	locker.SetAvailable(true)

	// charmdir is available, function will run
	ok, err = consumer.Run(func() error { return fmt.Errorf("yep") })
	c.Check(ok, jc.IsTrue)
	c.Check(err, gc.ErrorMatches, "yep")

	var g gate
	g.set("consumers have availability")

	beforeNA := make(chan struct{})
	afterNA := make(chan struct{})
	go func() {
		<-beforeNA
		locker.NotAvailable()
		close(afterNA)
		g.set("no availability for you")
	}()

	// charmdir available, Consumer.Run without error, confirm shared state.
	ok, err = consumer.Run(func() error {
		c.Check(g.get(), gc.Equals, "consumers have availability")
		return nil
	})
	c.Check(ok, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)

	// Locker wants to make charmdir NotAvailable during Consumer.Run, has to
	// wait, confirm shared state.
	ok, err = consumer.Run(func() error {
		close(beforeNA)
		select {
		case <-afterNA:
			c.Fatalf("Locker failed to keep charmdir locked during a Consumer.Run")
		case <-time.After(coretesting.ShortWait):
		}
		c.Check(g.get(), gc.Equals, "consumers have availability")
		return nil
	})
	c.Check(ok, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)

	// charmdir should now be unavailable, confirm shared state.
	select {
	case <-afterNA:
		c.Check(g.get(), gc.Equals, "no availability for you")
		ok, err = consumer.Run(func() error { return fmt.Errorf("nope") })
		c.Check(ok, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(coretesting.ShortWait):
		c.Fatal("timed out waiting for locker to revoke availability")
	}
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	worker, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer kill(worker)

	var state interface{}
	err = s.manifold.Output(worker, &state)
	c.Check(err.Error(), gc.Equals, "out should be a pointer to a charmdir.Consumer or a charmdir.Locker; is *interface {}")
	c.Check(state, gc.IsNil)
}
