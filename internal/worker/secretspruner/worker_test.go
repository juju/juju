// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner_test

import (
	"context"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/secretspruner"
	"github.com/juju/juju/internal/worker/secretspruner/mocks"
)

type workerSuite struct {
	testhelpers.IsolationSuite
	logger logger.Logger

	facade *mocks.MockSecretsFacade

	done      chan struct{}
	changedCh chan struct{}
}

func TestWorkerSuite(t *stdtesting.T) { tc.Run(t, &workerSuite{}) }
func (s *workerSuite) getWorkerNewer(c *tc.C) (func(string), *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.logger = loggertesting.WrapCheckLog(c)
	s.facade = mocks.NewMockSecretsFacade(ctrl)

	s.changedCh = make(chan struct{}, 1)
	s.done = make(chan struct{})
	s.facade.EXPECT().WatchRevisionsToPrune(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(s.changedCh), nil)

	start := func(expectedErr string) {
		w, err := secretspruner.NewWorker(secretspruner.Config{
			Logger:        s.logger,
			SecretsFacade: s.facade,
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(w, tc.NotNil)
		workertest.CheckAlive(c, w)
		s.AddCleanup(func(c *tc.C) {
			if expectedErr == "" {
				workertest.CleanKill(c, w)
			} else {
				err := workertest.CheckKilled(c, w)
				c.Assert(err, tc.ErrorMatches, expectedErr)
			}
		})
		s.waitDone(c)
	}
	return start, ctrl
}

func (s *workerSuite) waitDone(c *tc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *workerSuite) TestPrune(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.changedCh <- struct{}{}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		wg.Wait()
		close(s.done)
	}()

	s.facade.EXPECT().DeleteObsoleteUserSecretRevisions(gomock.Any()).DoAndReturn(func(context.Context) error {
		wg.Done()
		return nil
	})

	start("")
}
