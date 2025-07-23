// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crosscontroller"
	"github.com/juju/juju/core/crossmodel"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker/externalcontrollerupdater"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ExternalControllerUpdaterSuite{})

type ExternalControllerUpdaterSuite struct {
	coretesting.BaseSuite
	clock testclock.AdvanceableClock

	watcher *MockExternalControllerWatcherClientCloser
	client  *MockExternalControllerUpdaterClient
}

func (s *ExternalControllerUpdaterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcher = NewMockExternalControllerWatcherClientCloser(ctrl)
	s.client = NewMockExternalControllerUpdaterClient(ctrl)

	s.clock = testclock.NewDilatedWallClock(time.Millisecond)

	return ctrl
}

func (s *ExternalControllerUpdaterSuite) TestStartStop(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)

	s.client.EXPECT().WatchExternalControllers().Return(extCtrlWatcher, nil)

	w, err := externalcontrollerupdater.New(s.client, func(*api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
		return s.watcher, nil
	}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersStartStop(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().WatchExternalControllers().Return(extCtrlWatcher, nil)
	info := &crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Alias:         "alias",
		Addrs:         []string{"10.6.6.6"},
		CACert:        coretesting.CACert,
	}
	s.client.EXPECT().ExternalControllerInfo(coretesting.ControllerTag.Id()).Return(info, nil)

	started := make(chan struct{})

	infoWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.watcher.EXPECT().WatchControllerInfo().DoAndReturn(func() (corewatcher.NotifyWatcher, error) {
		return infoWatcher, nil
	})
	s.watcher.EXPECT().Close()

	w, err := externalcontrollerupdater.New(s.client, func(gotInfo *api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
		defer close(started)
		c.Assert(gotInfo, jc.DeepEquals, &api.Info{
			Addrs:  info.Addrs,
			Tag:    names.NewUserTag("jujuanonymous"),
			CACert: info.CACert,
		})
		return s.watcher, nil
	}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for watcher to start")
	}

	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().WatchExternalControllers().Return(extCtrlWatcher, nil)
	s.client.EXPECT().ExternalControllerInfo(coretesting.ControllerTag.Id()).Return(&crossmodel.ControllerInfo{}, nil)

	done := make(chan struct{})

	s.watcher.EXPECT().WatchControllerInfo().DoAndReturn(func() (corewatcher.NotifyWatcher, error) {
		return nil, errors.New("watcher error")
	})
	// Close should be called on error.
	s.watcher.EXPECT().Close().Do(func() {
		close(done)
	})

	w, err := externalcontrollerupdater.New(s.client, func(*api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
		return s.watcher, nil
	}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for watcher client to close")
	}

	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersErrorRestarts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().WatchExternalControllers().Return(extCtrlWatcher, nil)
	s.client.EXPECT().ExternalControllerInfo(coretesting.ControllerTag.Id()).Return(&crossmodel.ControllerInfo{}, nil)

	done := make(chan struct{})

	s.watcher.EXPECT().WatchControllerInfo().DoAndReturn(func() (corewatcher.NotifyWatcher, error) {
		return nil, errors.New("watcher error")
	})
	s.watcher.EXPECT().Close()

	w, err := externalcontrollerupdater.New(s.client, func(*api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
		return s.watcher, nil
	}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.clock.Advance(time.Minute)
	// After an error and a delay, restart the watcher.
	s.client.EXPECT().ExternalControllerInfo(coretesting.ControllerTag.Id()).Return(&crossmodel.ControllerInfo{}, nil)
	infoWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.watcher.EXPECT().WatchControllerInfo().DoAndReturn(func() (corewatcher.NotifyWatcher, error) {
		defer close(done)
		return infoWatcher, nil
	})
	s.watcher.EXPECT().Close()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for watcher to restart")
	}

	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().WatchExternalControllers().Return(extCtrlWatcher, nil)
	info := &crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Alias:         "alias",
		Addrs:         []string{"10.6.6.6"},
		CACert:        coretesting.CACert,
	}
	s.client.EXPECT().ExternalControllerInfo(coretesting.ControllerTag.Id()).Return(info, nil)

	notSupportedErr := &params.Error{Code: params.CodeAccessRequired}
	started := make(chan struct{})

	w, err := externalcontrollerupdater.New(s.client, func(*api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
		defer close(started)
		return nil, notSupportedErr
	}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for watcher")
	}

	// We expect the worker NOT to stop, only the spawned controllerWatcher
	// will finish. We can't check that here but if the notSupportedErr
	// is changed to a different error, the test fails because there is
	// no expected call to s.client.ExternalControllerInfo.
	s.clock.Advance(time.Minute)
	workertest.CheckAlive(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().WatchExternalControllers().Return(extCtrlWatcher, nil)
	info := crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Alias:         "alias",
		Addrs:         []string{"10.6.6.6"},
		CACert:        coretesting.CACert,
	}
	s.client.EXPECT().ExternalControllerInfo(coretesting.ControllerTag.Id()).Return(&info, nil)

	change := make(chan struct{}, 1)
	infoWatcher := watchertest.NewMockNotifyWatcher(change)

	s.watcher.EXPECT().WatchControllerInfo().DoAndReturn(func() (corewatcher.NotifyWatcher, error) {
		return infoWatcher, nil
	})
	s.watcher.EXPECT().Close()

	w, err := externalcontrollerupdater.New(s.client, func(gotInfo *api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
		return s.watcher, nil
	}, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	newInfo := &crosscontroller.ControllerInfo{
		Addrs:  []string{"10.6.6.7"},
		CACert: coretesting.CACert,
	}
	s.watcher.EXPECT().ControllerInfo().Return(newInfo, nil)

	done := make(chan struct{})

	updatedInfo := info
	updatedInfo.Addrs = newInfo.Addrs
	s.client.EXPECT().SetExternalControllerInfo(updatedInfo)

	// After processing the event, the watcher is closed and re-opened.
	s.watcher.EXPECT().Close()
	s.watcher.EXPECT().WatchControllerInfo().DoAndReturn(func() (corewatcher.NotifyWatcher, error) {
		defer close(done)
		return infoWatcher, nil
	})

	change <- struct{}{}

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for controller update")
	}

	workertest.CleanKill(c, w)
}
