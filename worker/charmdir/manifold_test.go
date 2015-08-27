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

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	worker, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer kill(worker)

	var consumer charmdir.Consumer
	err = s.manifold.Output(worker, &consumer)
	c.Check(err, jc.ErrorIsNil)

	// charmdir is not available, so function will not run
	err = consumer.Run(func() error { return fmt.Errorf("nope") })
	c.Check(err, gc.ErrorMatches, "charmdir not available")

	var locker charmdir.Locker
	err = s.manifold.Output(worker, &locker)
	c.Check(err, jc.ErrorIsNil)
	locker.SetAvailable(true)

	// charmdir is available, function will run
	err = consumer.Run(func() error { return fmt.Errorf("yep") })
	c.Check(err, gc.ErrorMatches, "yep")

	state := "consumers have availability"
	var mu sync.Mutex

	beforeUnavail := make(chan struct{})
	afterUnavail := make(chan struct{})
	go func() {
		<-beforeUnavail
		locker.SetAvailable(false)
		mu.Lock()
		state = "no availability for you"
		mu.Unlock()

		close(afterUnavail)
	}()

	// charmdir available, Consumer.Run without error, confirm shared state.
	err = consumer.Run(func() error {
		mu.Lock()
		c.Check(state, gc.Equals, "consumers have availability")
		mu.Unlock()
		return nil
	})
	c.Check(err, jc.ErrorIsNil)

	// Locker wants to make charmdir unavailable during Consumer.Run, has to
	// wait, confirm shared state.
	err = consumer.Run(func() error {
		close(beforeUnavail)
		select {
		case <-afterUnavail:
			c.Fatalf("Locker failed to keep charmdir locked during a Consumer.Run")
		case <-time.After(coretesting.ShortWait):
		}
		mu.Lock()
		c.Check(state, gc.Equals, "consumers have availability")
		mu.Unlock()
		return nil
	})
	c.Check(err, jc.ErrorIsNil)

	// charmdir should now be unavailable, confirm shared state.
	select {
	case <-afterUnavail:
		mu.Lock()
		c.Check(state, gc.Equals, "no availability for you")
		mu.Unlock()

		err = consumer.Run(func() error { return fmt.Errorf("nope") })
		c.Check(err, gc.ErrorMatches, "charmdir not available")
	case <-time.After(coretesting.ShortWait):
		c.Fatal("timed out waiting for locker to revoke availability")
	}
}

func (s *ManifoldSuite) TestConcurrentConsumers(c *gc.C) {
	worker, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer kill(worker)

	var consumer charmdir.Consumer
	err = s.manifold.Output(worker, &consumer)
	c.Check(err, jc.ErrorIsNil)

	var locker charmdir.Locker
	err = s.manifold.Output(worker, &locker)
	c.Check(err, jc.ErrorIsNil)

	nconsumers := 10
	before := make(chan struct{})
	after := make(chan struct{}, nconsumers)
	for i := 0; i < nconsumers; i++ {
		go func() {
			<-before
			err := consumer.Run(func() error {
				after <- struct{}{}
				return nil
			})
			c.Check(err, jc.ErrorIsNil)
		}()
	}

	locker.SetAvailable(true)
	close(before)

	for i := 0; i < nconsumers; i++ {
		select {
		case <-after:
		case <-time.After(coretesting.ShortWait):
			c.Fatal("timed out waiting to confirm consumer worker exit")
		}
	}
}

func (s *ManifoldSuite) TestConcurrentLockers(c *gc.C) {
	worker, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer kill(worker)

	var locker charmdir.Locker
	err = s.manifold.Output(worker, &locker)
	c.Check(err, jc.ErrorIsNil)

	nlockers := 10
	ch := make(chan struct{}, nlockers)
	for i := 0; i < nlockers; i++ {
		go func() {
			locker.SetAvailable(true)
			locker.SetAvailable(false)
			ch <- struct{}{}
		}()
	}
	for i := 0; i < nlockers; i++ {
		select {
		case <-ch:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting to confirm locker worker exit")
		}
	}
}

func (s *ManifoldSuite) TestConcurrentLockersConsumers(c *gc.C) {
	worker, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)
	defer kill(worker)

	var consumer charmdir.Consumer
	err = s.manifold.Output(worker, &consumer)
	c.Check(err, jc.ErrorIsNil)

	var locker charmdir.Locker
	err = s.manifold.Output(worker, &locker)
	c.Check(err, jc.ErrorIsNil)

	nworkers := 20
	before := make(chan struct{})
	after := make(chan struct{}, nworkers)
	for i := 0; i < nworkers; i++ {
		var f func()
		if i%2 == 0 {
			f = func() {
				<-before
				err := consumer.Run(func() error {
					after <- struct{}{}
					return nil
				})
				if err == charmdir.ErrNotAvailable {
					after <- struct{}{}
				}
			}
		} else {
			f = func() {
				<-before
				if i%3 == 0 {
					locker.SetAvailable(false)
					locker.SetAvailable(true)
				}
				after <- struct{}{}
			}
		}
		go f()
	}

	locker.SetAvailable(true)
	close(before)

	for i := 0; i < nworkers; i++ {
		select {
		case <-after:
		case <-time.After(coretesting.ShortWait):
			c.Fatal("timed out waiting to confirm consumer worker exit")
		}
	}
}
