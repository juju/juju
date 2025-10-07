// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/consumerunitrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/remoterelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/remoteunitrelations"
)

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

type workerSuite struct {
	baseSuite

	modelUUID model.UUID
}

func (s *workerSuite) TestWorkerKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.crossModelService.EXPECT().WatchRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			defer close(done)
			return watchertest.NewMockNotifyWatcher(make(chan struct{})), nil
		})

	w := s.newWorker(c, nil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRemoteApplications to be called")
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

	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{{
				ApplicationUUID: appUUID,
				ApplicationName: "foo",
				Life:            life.Alive,
				OfferUUID:       "offer-uuid",
				ConsumeVersion:  0,
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

	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{{
				ApplicationUUID: appUUID,
				ApplicationName: "foo",
				Life:            life.Alive,
				OfferUUID:       "offer-uuid",
				ConsumeVersion:  0,
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

func (s *workerSuite) TestRemoteApplicationsOfferChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := tc.Must(c, uuid.NewUUID).String()

	ch := make(chan struct{})

	exp := s.crossModelService.EXPECT()
	exp.WatchRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(ch), nil
		})

	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{{
				ApplicationUUID: appUUID,
				ApplicationName: "foo",
				Life:            life.Alive,
				OfferUUID:       "offer-uuid",
				ConsumeVersion:  0,
			}}, nil
		})

	exp.GetRemoteApplicationOfferers(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
			return []crossmodelrelation.RemoteApplicationOfferer{{
				ApplicationUUID: appUUID,
				ApplicationName: "foo",
				Life:            life.Alive,
				OfferUUID:       "offer-uuid",
				ConsumeVersion:  1,
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

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.modelUUID = modeltesting.GenModelUUID(c)

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

			return &testRemoteApplicationWorker{
				reportableWorker: reportableWorker{Worker: workertest.NewErrorWorker(nil)},
				offerUUID:        config.OfferUUID,
				consumeVersion:   config.ConsumeVersion,
				applicationName:  config.ApplicationName,
			}, nil
		},
		NewConsumerUnitRelationsWorker: func(c consumerunitrelations.Config) (consumerunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererUnitRelationsWorker: func(c remoteunitrelations.Config) (remoteunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewOffererRelationsWorker: func(c remoterelations.Config) (remoterelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		Clock:  clock.WallClock,
		Logger: s.logger,
	})
	c.Assert(err, tc.ErrorIsNil)
	return w.(*Worker)
}

type testRemoteApplicationWorker struct {
	reportableWorker

	offerUUID       string
	consumeVersion  int
	applicationName string
}

var _ RemoteApplicationWorker = (*testRemoteApplicationWorker)(nil)

func (w *testRemoteApplicationWorker) OfferUUID() string {
	return w.offerUUID
}

func (w *testRemoteApplicationWorker) ConsumeVersion() int {
	return w.consumeVersion
}

func (w *testRemoteApplicationWorker) ApplicationName() string {
	return w.applicationName
}
