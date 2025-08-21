// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
)

func TestWorkerSuite(t *stdtesting.T) {
	tc.Run(t, &workerSuite{})
}

type workerSuite struct {
	baseSuite

	modelUUID model.UUID
}

func (s *workerSuite) TestWorkerKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.remoteRelationsFacade.EXPECT().WatchRemoteApplications(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.StringsWatcher, error) {
			defer close(done)
			return watchertest.NewMockStringsWatcher(make(chan []string)), nil
		})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRemoteApplications to be called")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNoRemoteApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []string)

	appIDs := []string{"app1"}

	exp := s.remoteRelationsFacade.EXPECT()
	exp.WatchRemoteApplications(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(ch), nil
		})

	done := make(chan struct{})
	exp.RemoteApplications(gomock.Any(), appIDs).
		DoAndReturn(func(ctx context.Context, s []string) ([]params.RemoteApplicationResult, error) {
			defer close(done)
			return []params.RemoteApplicationResult{{
				Error: &params.Error{Code: params.CodeNotFound, Message: "not found"},
			}}, nil
		})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- appIDs:
	case <-c.Context().Done():
		c.Fatalf("timed out pushing application IDs to WatchRemoteApplications")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRemoteApplications to be called")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.modelUUID = modeltesting.GenModelUUID(c)

	return ctrl
}

func (s *workerSuite) newWorker(c *tc.C) *Worker {
	w, err := NewWorker(Config{
		ModelUUID:                  s.modelUUID.String(),
		RelationsFacade:            s.remoteRelationsFacade,
		RemoteRelationClientGetter: s.remoteRelationClientGetter,
		NewRemoteApplicationWorker: func(config RemoteApplicationConfig) (ReportableWorker, error) {
			return &testRemoteApplicationWorker{
				reportableWorker: reportableWorker{Worker: workertest.NewErrorWorker(nil)},
				offerUUID:        config.OfferUUID,
				applicationID:    config.ApplicationID,
				applicationName:  config.ApplicationName,
			}, nil
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
	applicationID   string
	applicationName string
}

func (w *testRemoteApplicationWorker) OfferUUID() string {
	return w.offerUUID
}

func (w *testRemoteApplicationWorker) ApplicationID() string {
	return w.applicationID
}

func (w *testRemoteApplicationWorker) ApplicationName() string {
	return w.applicationName
}
