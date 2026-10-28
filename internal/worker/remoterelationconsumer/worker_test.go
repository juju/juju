// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/consumerunitrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/offererrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/offererunitrelations"
)

func TestWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &workerSuite{})
	})
}

type workerSuite struct {
	baseSuite

	modelUUID model.UUID
}

func (s *workerSuite) TestWorkerKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	started := make(chan struct{})
	s.crossModelService.EXPECT().WatchRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
		})
	s.crossModelService.EXPECT().WatchDyingModel(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			defer close(started)
			return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
		})

	w := s.newWorker(c, nil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-started:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker startup to complete")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestRemoteApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, uuid.NewUUID).String()

	ch := make(chan struct{})

	exp := s.crossModelService.EXPECT()
	exp.WatchRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(ch), nil
		})
	exp.WatchDyingModel(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
		})

	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{{
				ApplicationUUID: appUUID,
				ApplicationName: "foo",
				Life:            life.Alive,
				OfferUUID:       "offer-uuid",
			}}, nil
		})

	started := make(chan string, 1)

	w := s.newWorker(c, started)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out pushing application UUIDs to WatchRemoteApplications")
	}

	select {
	case appName := <-started:
		c.Check(appName, tc.Equals, "foo")
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for application to be started")
	}

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{appUUID})

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestRemoteApplicationsGone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, uuid.NewUUID).String()

	ch := make(chan struct{})

	exp := s.crossModelService.EXPECT()
	exp.WatchRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(ch), nil
		})
	exp.WatchDyingModel(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
		})

	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{{
				ApplicationUUID: appUUID,
				ApplicationName: "foo",
				Life:            life.Alive,
				OfferUUID:       "offer-uuid",
			}}, nil
		})

	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{}, nil
		})

	started := make(chan string, 1)

	w := s.newWorker(c, started)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out pushing application UUIDs to WatchRemoteApplications")
	}

	select {
	case appName := <-started:
		c.Check(appName, tc.Equals, "foo")
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for application to be started")
	}

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out pushing application UUIDs to WatchRemoteApplications")
	}

	waitForEmptyRunner(c, w.runner)

	workertest.CleanKill(c, w)
}

// TestWorkerNoRestartOnOffererDeadErr ensures the runner does not restart a
// local consumer worker that exited with the terminal
// [RemoteApplicationOffererDeadErr]. The worker schedules the remote
// application offerer removal before exiting; a restart would re-schedule
// the removal jobs on every restart delay, forever.
func (s *workerSuite) TestWorkerNoRestartOnOffererDeadErr(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, uuid.NewUUID).String()

	ch := make(chan struct{})

	exp := s.crossModelService.EXPECT()
	exp.WatchRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(ch), nil
		})
	exp.WatchDyingModel(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
		})
	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{{
				ApplicationUUID: appUUID,
				ApplicationName: "foo",
				Life:            life.Alive,
				OfferUUID:       "offer-uuid",
			}}, nil
		}).AnyTimes()

	clk := NewMockClock(ctrl)
	now := time.Now()
	clk.EXPECT().Now().Return(now).AnyTimes()
	after := make(chan time.Time, 1)
	clk.EXPECT().After(gomock.Any()).Return(after).AnyTimes()

	started := make(chan string, 2)
	w, err := NewWorker(Config{
		ModelUUID:                  s.modelUUID,
		CrossModelService:          s.crossModelService,
		RemoteRelationClientGetter: s.remoteRelationClientGetter,
		NewLocalConsumerWorker: func(config LocalConsumerWorkerConfig) (ReportableWorker, error) {
			started <- config.ApplicationName
			// The worker exits immediately with the terminal error,
			// as it does once the offer is gone.
			return reportableWorker{Worker: workertest.NewDeadWorker(RemoteApplicationOffererDeadErr)}, nil
		},
		NewConsumerUnitRelationsWorker: func(c consumerunitrelations.Config) (consumerunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererUnitRelationsWorker: func(c offererunitrelations.Config) (offererunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererRelationsWorker: func(c offererrelations.Config) (offererrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		Clock:  clk,
		Logger: s.logger,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out pushing application UUIDs to WatchRemoteApplications")
	}

	select {
	case <-started:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for application to be started")
	}

	// Allow the worker to exit, then fire any pending restart delay timers.
	// The runner must not restart the worker, because it exited with the
	// terminal dead error.
	after <- now
	select {
	case name := <-started:
		c.Fatalf("local consumer worker %q restarted after terminal dead error", name)
	case <-time.After(testing.ShortWait):
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.modelUUID = tc.Must0(c, model.NewUUID)

	return ctrl
}

func (s *workerSuite) newWorker(c *tc.C, started chan<- string) *Worker {
	w, err := NewWorker(Config{
		ModelUUID:                  s.modelUUID,
		CrossModelService:          s.crossModelService,
		RemoteRelationClientGetter: s.remoteRelationClientGetter,
		NewLocalConsumerWorker: func(config LocalConsumerWorkerConfig) (ReportableWorker, error) {
			defer func() {
				started <- config.ApplicationName
			}()

			return &testOffererApplicationWorker{
				reportableWorker: reportableWorker{Worker: workertest.NewErrorWorker(nil)},
				offerUUID:        config.OfferUUID,
				consumeVersion:   config.ConsumeVersion,
				applicationName:  config.ApplicationName,
			}, nil
		},
		NewConsumerUnitRelationsWorker: func(c consumerunitrelations.Config) (consumerunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererUnitRelationsWorker: func(c offererunitrelations.Config) (offererunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererRelationsWorker: func(c offererrelations.Config) (offererrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		Clock:  clock.WallClock,
		Logger: s.logger,
	})
	c.Assert(err, tc.ErrorIsNil)
	return w.(*Worker)
}

type testOffererApplicationWorker struct {
	reportableWorker

	offerUUID       string
	consumeVersion  int
	applicationName string
}

var _ OffererApplicationWorker = (*testOffererApplicationWorker)(nil)

func (w *testOffererApplicationWorker) ConsumeVersion() int {
	return w.consumeVersion
}

func (w *testOffererApplicationWorker) PublishModelDying(ctx context.Context) error {
	return nil
}
