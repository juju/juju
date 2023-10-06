// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"
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
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStartup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, _, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSighup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	s.sendSignal(c, notify)
	s.ensureReload(c, states)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSighupMultipleTimes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	for i := 0; i < 10; i++ {
		s.sendSignal(c, notify)
		s.ensureReload(c, states)
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSighupAfterDeath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	workertest.CleanKill(c, w)

	// We should not receive a reload signal after the worker has died.
	s.sendSignal(c, notify)

	select {
	case state := <-states:
		c.Fatalf("should not have received state %q", state)
	case <-time.After(testing.ShortWait * 10):
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)
	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) (worker.Worker, chan struct{}, chan string) {
	// Buffer the channel, so we don't drop signals if we're not ready.
	states := make(chan string, 10)
	// Buffer the channel, so we don't miss signals if we're not ready.
	notify := make(chan struct{}, 1)
	w, err := newWorker(WorkerConfig{
		Logger: s.logger,
		Notify: func(ctx context.Context, ch chan os.Signal) {
			go func() {
				for {
					select {
					case <-notify:
						select {
						case ch <- syscall.SIGHUP:
						case <-ctx.Done():
							return
						}

					case <-ctx.Done():
						return
					}
				}
			}()
		},
	}, states)
	c.Assert(err, jc.ErrorIsNil)
	return w, notify, states
}

func (s *workerSuite) ensureStartup(c *gc.C, states chan string) {
	select {
	case state := <-states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *workerSuite) ensureReload(c *gc.C, states chan string) {
	select {
	case state := <-states:
		c.Assert(state, gc.Equals, stateReload)
	case <-time.After(testing.ShortWait * 100):
		c.Fatalf("timed out waiting for reload")
	}
}

func (s *workerSuite) sendSignal(c *gc.C, notify chan struct{}) {
	select {
	case notify <- struct{}{}:
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out sending signal")
	}
}
