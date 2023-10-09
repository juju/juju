// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner_test

import (
	"sync"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/secretspruner"
	"github.com/juju/juju/worker/secretspruner/mocks"
)

type workerSuite struct {
	testing.IsolationSuite
	logger loggo.Logger

	facade *mocks.MockSecretsFacade

	done      chan struct{}
	changedCh chan []string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) getWorkerNewer(c *gc.C, calls ...*gomock.Call) (func(string), *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.logger = loggo.GetLogger("test")
	s.facade = mocks.NewMockSecretsFacade(ctrl)

	s.changedCh = make(chan []string, 1)
	s.done = make(chan struct{})
	s.facade.EXPECT().WatchRevisionsToPrune().Return(watchertest.NewMockStringsWatcher(s.changedCh), nil)

	start := func(expectedErr string) {
		w, err := secretspruner.NewWorker(secretspruner.Config{
			Logger:        s.logger,
			SecretsFacade: s.facade,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(w, gc.NotNil)
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
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *workerSuite) TestPrune(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	var revisions []string
	revisions = append(revisions, uri1.String()+"/1")
	revisions = append(revisions, uri2.String()+"/1")
	revisions = append(revisions, uri2.String()+"/2")
	revisions = append(revisions, uri3.String()+"/1")
	revisions = append(revisions, uri3.String()+"/2")
	revisions = append(revisions, uri3.String()+"/3")
	s.changedCh <- revisions

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		wg.Wait()
		close(s.done)
	}()

	s.facade.EXPECT().DeleteRevisions(uri1, 1).DoAndReturn(func(*coresecrets.URI, ...int) error {
		wg.Done()
		return nil
	})
	s.facade.EXPECT().DeleteRevisions(uri2, 1, 2).DoAndReturn(func(*coresecrets.URI, ...int) error {
		wg.Done()
		return nil
	})
	s.facade.EXPECT().DeleteRevisions(uri3, 1, 2, 3).DoAndReturn(func(*coresecrets.URI, ...int) error {
		wg.Done()
		return nil
	})

	start("")
}
