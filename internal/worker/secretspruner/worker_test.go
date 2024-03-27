// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner_test

import (
	"sync"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker/secretspruner"
	"github.com/juju/juju/internal/worker/secretspruner/mocks"
)

type workerSuite struct {
	testing.IsolationSuite
	logger loggo.Logger

	facade *mocks.MockSecretsFacade

	done      chan struct{}
	changedCh chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) getWorkerNewer(c *gc.C, calls ...*gomock.Call) (func(string), *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.logger = loggo.GetLogger("test")
	s.facade = mocks.NewMockSecretsFacade(ctrl)

	s.changedCh = make(chan struct{}, 1)
	s.done = make(chan struct{})
	s.facade.EXPECT().WatchRevisionsToPrune().Return(watchertest.NewMockNotifyWatcher(s.changedCh), nil)

	start := func(expectedErr string) {
		w, err := secretspruner.NewWorker(secretspruner.Config{
			Logger:        s.logger,
			SecretsFacade: s.facade,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(w, gc.NotNil)
		workertest.CheckAlive(c, w)
		s.AddCleanup(func(c *gc.C) {
			if expectedErr == "" {
				workertest.CleanKill(c, w)
			} else {
				err := workertest.CheckKilled(c, w)
				c.Assert(err, gc.ErrorMatches, expectedErr)
			}
		})
		s.waitDone(c)
	}
	return start, ctrl
}

func (s *workerSuite) waitDone(c *gc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *workerSuite) TestPrune(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.changedCh <- struct{}{}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		wg.Wait()
		close(s.done)
	}()

	s.facade.EXPECT().DeleteObsoleteUserSecrets().DoAndReturn(func() error {
		wg.Done()
		return nil
	})

	start("")
}
