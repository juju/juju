// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package voyeur_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/voyeur"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	workervoyeur "github.com/juju/juju/worker/voyeur"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	value    *voyeur.Value
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.value = voyeur.NewValue(0)
	s.manifold = workervoyeur.Manifold(workervoyeur.ManifoldConfig{
		Value: s.value,
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{})
}

func (s *ManifoldSuite) TestNilValue(c *gc.C) {
	manifold := workervoyeur.Manifold(workervoyeur.ManifoldConfig{})
	_, err := manifold.Start(nilGetResource)
	c.Assert(err, gc.ErrorMatches, "nil Value .+")
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w, err := s.manifold.Start(nilGetResource)
	c.Assert(err, jc.ErrorIsNil)
	checkStop(c, w)
}

func (s *ManifoldSuite) TestBounceOnChange(c *gc.C) {
	w, err := s.manifold.Start(nilGetResource)
	c.Assert(err, jc.ErrorIsNil)
	checkNotExiting(c, w)
	s.value.Set(true)
	checkExitsWithError(c, w, dependency.ErrBounce)

	w, err = s.manifold.Start(nilGetResource)
	c.Assert(err, jc.ErrorIsNil)
	checkNotExiting(c, w)
	s.value.Set(false) // The actual value doesn't matter.
	checkExitsWithError(c, w, dependency.ErrBounce)
}

func checkStop(c *gc.C, w worker.Worker) {
	err := worker.Stop(w)
	c.Check(err, jc.ErrorIsNil)
}

func checkNotExiting(c *gc.C, w worker.Worker) {
	exited := make(chan bool)
	go func() {
		w.Wait()
		close(exited)
	}()

	select {
	case <-exited:
		c.Fatal("worker exited unexpectedly")
	case <-time.After(coretesting.ShortWait):
		// Worker didn't exit (good)
	}
}

func checkExitsWithError(c *gc.C, w worker.Worker, expectedErr error) {
	errCh := make(chan error)
	go func() {
		errCh <- w.Wait()
	}()
	select {
	case err := <-errCh:
		c.Check(err, gc.Equals, expectedErr)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to exit")
	}
}

func nilGetResource(name string, out interface{}) error {
	return nil
}
