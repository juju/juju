// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/watcher"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/testing"
	workermocks "github.com/juju/juju/worker/mocks"
	"github.com/juju/juju/worker/spacenamer"
	"github.com/juju/juju/worker/spacenamer/mocks"
)

type fakeWatcher struct {
	worker.Worker
	ch <-chan struct{}
}

func (w *fakeWatcher) Changes() watcher.NotifyChannel {
	return w.ch
}

type workerSuite struct {
	testing.BaseSuite

	notifyWorker *workermocks.MockWorker
	api          *mocks.MockSpaceNamerAPI
	logger       *mocks.MockLogger

	config spacenamer.WorkerConfig
	done   chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.done = make(chan struct{})
}

func (s *workerSuite) TestWorker(c *gc.C) {
	defer s.setup(c).Finish()

	s.notify(1)
	s.expectSetDefaultSpaceName(nil)

	w, err := spacenamer.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.cleanKill(c, w)
}

func (s *workerSuite) TestWorkerWatcherFail(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectWatchDefaultSpaceConfigFail()

	w, err := spacenamer.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKill(c, w)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *workerSuite) TestWorkerSetNameFail(c *gc.C) {
	defer s.setup(c).Finish()

	s.notify(2)
	s.expectSetDefaultSpaceName(errors.New("fail me"))
	s.expectSetDefaultSpaceName(nil)
	s.expectLoggerError()

	// Keep running even if SetDefaultSpaceName fails.
	w, err := spacenamer.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.cleanKill(c, w)
}

func (s *workerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.api = mocks.NewMockSpaceNamerAPI(ctrl)
	s.logger = mocks.NewMockLogger(ctrl)
	s.notifyWorker = workermocks.NewMockWorker(ctrl)

	s.config = spacenamer.WorkerConfig{
		API:    s.api,
		Logger: s.logger,
	}

	return ctrl
}

func (s *workerSuite) expectSetDefaultSpaceName(err error) {
	s.api.EXPECT().SetDefaultSpaceName().Return(err)
}

func (s *workerSuite) expectWatchDefaultSpaceConfigFail() {
	s.api.EXPECT().WatchDefaultSpaceConfig().Return(nil, errors.NotValidf("fail me"))
}

func (s *workerSuite) expectLoggerError() {
	s.logger.EXPECT().Errorf("Received error setting Default Space Name: %s", gomock.Any())
}

// notify returns a suite behaviour that will cause the model config watcher
// to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notify(times int) {
	ch := make(chan struct{})

	go func() {
		for i := 0; i < times; i++ {
			ch <- struct{}{}
		}
		close(s.done)
	}()

	s.notifyWorker.EXPECT().Kill().AnyTimes()
	s.notifyWorker.EXPECT().Wait().Return(nil).AnyTimes()

	s.api.EXPECT().WatchDefaultSpaceConfig().Return(
		&fakeWatcher{
			Worker: s.notifyWorker,
			ch:     ch,
		}, nil)
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *workerSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
}
