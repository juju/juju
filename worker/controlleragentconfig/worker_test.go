// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"os"
	"syscall"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	states chan string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStartup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()
}

func (s *workerSuite) TestSighup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)
	s.sendSignal(c)
	s.ensureReload(c)

	w.Kill()
}

func (s *workerSuite) TestSighupMultipleTimes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	for i := 0; i < 10; i++ {
		s.sendSignal(c)
		s.ensureReload(c)
	}

	w.Kill()
}

func (s *workerSuite) TestSighupAfterDeath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)

	// We should not receive a reload signal after the worker has died.
	s.sendSignal(c)

	select {
	case state := <-s.states:
		c.Fatalf("should not have received state %q", state)
	case <-time.After(testing.ShortWait * 10):
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	s.states = make(chan string)

	ctrl := s.baseSuite.setupMocks(c)

	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Logger: s.logger,
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *workerSuite) ensureReload(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateReload)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for reload")
	}
}

func (s *workerSuite) sendSignal(c *gc.C) {
	p, err := os.FindProcess(os.Getpid())
	c.Assert(err, jc.ErrorIsNil)

	err = p.Signal(syscall.SIGHUP)
	c.Assert(err, jc.ErrorIsNil)
}
