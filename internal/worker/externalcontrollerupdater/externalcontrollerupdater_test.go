// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crosscontroller"
	"github.com/juju/juju/core/crossmodel"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	externalcontrollererrors "github.com/juju/juju/domain/externalcontroller/errors"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestExternalControllerUpdaterSuite(t *testing.T) {
	tc.Run(t, &ExternalControllerUpdaterSuite{})
}

type ExternalControllerUpdaterSuite struct {
	coretesting.BaseSuite
	clock testclock.AdvanceableClock

	watcher *MockExternalControllerWatcherClientCloser
	client  *MockExternalControllerService
}

func (s *ExternalControllerUpdaterSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcher = NewMockExternalControllerWatcherClientCloser(ctrl)
	s.client = NewMockExternalControllerService(ctrl)

	s.clock = testclock.NewDilatedWallClock(time.Millisecond)

	return ctrl
}

func (s *ExternalControllerUpdaterSuite) TestStartStop(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)

	w, err := New(s.client, func(context.Context, *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		return s.watcher, "10.0.0.1", nil
	}, s.clock, nil)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersStartStop(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)
	info := &crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "alias",
		Addrs:          []string{"10.6.6.6"},
		CACert:         coretesting.CACert,
	}
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).Return(info, nil)

	started := make(chan struct{})

	infoWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.watcher.EXPECT().WatchControllerInfo(gomock.Any()).DoAndReturn(func(context.Context) (corewatcher.NotifyWatcher, error) {
		return infoWatcher, nil
	})

	finalise := make(chan struct{})
	s.watcher.EXPECT().Close().DoAndReturn(func() error {
		close(finalise)
		return nil
	})

	w, err := New(s.client, func(_ context.Context, gotInfo *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		defer close(started)
		c.Assert(gotInfo, tc.DeepEquals, &api.Info{
			Addrs:  info.Addrs,
			Tag:    names.NewUserTag("jujuanonymous"),
			CACert: info.CACert,
		})
		return s.watcher, "10.0.0.1", nil
	}, s.clock, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-started:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for watcher to start")
	}

	workertest.CleanKill(c, w)

	select {
	case <-finalise:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for final call")
	}
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).Return(&crossmodel.ControllerInfo{}, nil)

	done := make(chan struct{})

	s.watcher.EXPECT().WatchControllerInfo(gomock.Any()).DoAndReturn(func(context.Context) (corewatcher.NotifyWatcher, error) {
		return nil, errors.New("watcher error")
	})
	// Close should be called on error.
	s.watcher.EXPECT().Close().DoAndReturn(func() error {
		close(done)
		return nil
	})

	w, err := New(s.client, func(context.Context, *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		return s.watcher, "10.0.0.1", nil
	}, s.clock, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for watcher client to close")
	}

	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersErrorRestarts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).Return(&crossmodel.ControllerInfo{}, nil)

	done := make(chan struct{})

	s.watcher.EXPECT().WatchControllerInfo(gomock.Any()).DoAndReturn(func(context.Context) (corewatcher.NotifyWatcher, error) {
		return nil, errors.New("watcher error")
	})
	s.watcher.EXPECT().Close()

	w, err := New(s.client, func(context.Context, *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		return s.watcher, "10.0.0.1", nil
	}, s.clock, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.clock.Advance(time.Minute)
	// After an error and a delay, restart the watcher.
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).Return(&crossmodel.ControllerInfo{}, nil)
	infoWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.watcher.EXPECT().WatchControllerInfo(gomock.Any()).DoAndReturn(func(context.Context) (corewatcher.NotifyWatcher, error) {
		defer close(done)
		return infoWatcher, nil
	})

	finalise := make(chan struct{})
	s.watcher.EXPECT().Close().DoAndReturn(func() error {
		close(finalise)
		return nil
	})

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for watcher to restart")
	}

	workertest.CleanKill(c, w)

	select {
	case <-finalise:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for final call")
	}
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)
	info := &crossmodel.ControllerInfo{}
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).Return(info, nil)

	notSupportedErr := &params.Error{Code: params.CodeNotSupported}
	watcherReady := make(chan struct{})
	watcherFetched := make(chan struct{})

	w, err := New(s.client, func(context.Context, *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		close(watcherReady)
		select {
		case <-watcherFetched:
		case <-time.After(coretesting.LongWait):
			c.Error("timed out waiting for watcher to be fetched")
		}
		return nil, "", notSupportedErr
	}, s.clock, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Here we synchronise access to the controllerWatcher worker started
	// by the runner in the updaterWorker. Fetch the single controllerWatcher
	// worker from the the list of running workers before it is killed and
	// removed, then check that it is killed with the expected error.
	select {
	case <-watcherReady:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for watcher to be ready")
	}
	updater, _ := w.(*updaterWorker)
	c.Assert(updater, tc.NotNil)
	runner := updater.runner
	n := runner.WorkerNames()
	c.Assert(n, tc.HasLen, 1)
	controllerWatcher, err := runner.Worker(n[0], nil)
	c.Assert(err, tc.IsNil)
	close(watcherFetched)

	err = workertest.CheckKilled(c, controllerWatcher)
	c.Assert(err, tc.IsNil)

	workertest.CheckAlive(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)
	info := crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "alias",
		Addrs:          []string{"10.6.6.6"},
		CACert:         coretesting.CACert,
	}
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).Return(&info, nil)

	change := make(chan struct{}, 1)
	infoWatcher := watchertest.NewMockNotifyWatcher(change)

	s.watcher.EXPECT().WatchControllerInfo(gomock.Any()).DoAndReturn(func(ctx context.Context) (corewatcher.NotifyWatcher, error) {
		return infoWatcher, nil
	})
	s.watcher.EXPECT().Close()

	w, err := New(s.client, func(context.Context, *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		return s.watcher, "10.0.0.1", nil
	}, s.clock, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	newInfo := &crosscontroller.ControllerInfo{
		Addrs:  []string{"10.6.6.7"},
		CACert: coretesting.CACert,
	}
	s.watcher.EXPECT().ControllerInfo(gomock.Any()).Return(newInfo, nil)

	done := make(chan struct{})

	updatedInfo := info
	updatedInfo.Addrs = newInfo.Addrs
	s.client.EXPECT().UpdateExternalController(gomock.Any(), updatedInfo)

	// After processing the event, the watcher is closed and re-opened.
	s.watcher.EXPECT().Close()
	s.watcher.EXPECT().WatchControllerInfo(gomock.Any()).DoAndReturn(func(context.Context) (corewatcher.NotifyWatcher, error) {
		defer close(done)
		return infoWatcher, nil
	})

	change <- struct{}{}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for controller update")
	}

	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllersNoChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)
	info := crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "alias",
		Addrs:          []string{"10.6.6.6", "10.6.6.7"},
		CACert:         coretesting.CACert,
	}
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).Return(&info, nil)

	change := make(chan struct{}, 1)
	infoWatcher := watchertest.NewMockNotifyWatcher(change)

	s.watcher.EXPECT().WatchControllerInfo(gomock.Any()).DoAndReturn(func(ctx context.Context) (corewatcher.NotifyWatcher, error) {
		return infoWatcher, nil
	})
	s.watcher.EXPECT().Close()

	done := make(chan struct{})
	w, err := New(s.client, func(ctx context.Context, gotInfo *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		return s.watcher, "10.0.0.1", nil
	}, s.clock, func() {
		close(done)
	})
	c.Assert(err, tc.ErrorIsNil)

	defer workertest.CleanKill(c, w)

	newInfo := &crosscontroller.ControllerInfo{
		// Re-order the addresses to ensure order doesn't matter.
		Addrs:  []string{"10.6.6.7", "10.6.6.6"},
		CACert: coretesting.CACert,
	}
	s.watcher.EXPECT().ControllerInfo(gomock.Any()).Return(newInfo, nil)

	change <- struct{}{}

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for controller watcher")
	}

	workertest.CleanKill(c, w)
}

func (s *ExternalControllerUpdaterSuite) TestControllerWatcherNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string, 1)
	extCtrlWatcher := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{coretesting.ControllerTag.Id()}

	s.client.EXPECT().Watch(gomock.Any()).Return(extCtrlWatcher, nil)

	notFoundCalled := make(chan struct{})
	s.client.EXPECT().Controller(gomock.Any(), coretesting.ControllerTag.Id()).DoAndReturn(func(ctx context.Context, uuid string) (*crossmodel.ControllerInfo, error) {
		close(notFoundCalled)
		// Return the same wrapped form the external-controller domain service
		// produces, so this test exercises the production error shape rather than
		// the bare sentinel.
		return nil, fmt.Errorf("%w for uuid %q", externalcontrollererrors.NotFound, uuid)
	})

	w, err := New(s.client, func(context.Context, *api.Info) (ExternalControllerWatcherClientCloser, string, error) {
		return s.watcher, "10.0.0.1", nil
	}, s.clock, nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-notFoundCalled:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for not-found lookup")
	}

	// The inner controllerWatcher must exit cleanly with nil after a NotFound
	// result. The runner removes a child worker from its workers map only when
	// it finishes with no error and is not in the process of being stopped.
	// Polling for the child's removal therefore proves benign-completion
	// semantics rather than restart-after-error.
	uw := w.(*updaterWorker)
	tagID := coretesting.ControllerTag.Id()
	for {
		if !slices.Contains(uw.runner.WorkerNames(), tagID) {
			break
		}
		select {
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for inner controllerWatcher %q to exit; runner workers: %v", tagID, uw.runner.WorkerNames())
		case <-time.After(time.Millisecond):
		}
	}

	workertest.CleanKill(c, w)
}
