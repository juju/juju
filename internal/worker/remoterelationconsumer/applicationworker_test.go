// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/localunitrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/remoterelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/remoteunitrelations"
	"github.com/juju/juju/rpc/params"
)

func TestApplicationWorker(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &applicationWorkerSuite{})
}

type applicationWorkerSuite struct {
	baseSuite

	applicationUUID application.UUID
	remoteModelUUID string
	offerUUID       string
	macaroon        *macaroon.Macaroon
}

func (s *applicationWorkerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationUUID = tc.Must(c, application.NewID)
	s.remoteModelUUID = tc.Must(c, model.NewUUID).String()
	s.offerUUID = tc.Must(c, uuid.NewUUID).String()

	s.macaroon = newMacaroon(c, "test")
}

func (s *applicationWorkerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newApplicationConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.newApplicationConfig(c)
	cfg.CrossModelService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.RemoteRelationClientGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.OfferUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.ApplicationName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.ApplicationUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.LocalModelUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.RemoteModelUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.Macaroon = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.NewLocalUnitRelationsWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.NewRemoteUnitRelationsWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.NewRemoteRelationsWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newApplicationConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *applicationWorkerSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {
			ch := make(chan []string)
			return watchertest.NewMockStringsWatcher(ch), nil
		})

	s.remoteRelationClientGetter.EXPECT().
		GetRemoteRelationClient(gomock.Any(), s.remoteModelUUID).
		Return(s.remoteModelRelationClient, nil)

	s.remoteModelRelationClient.EXPECT().
		WatchOfferStatus(gomock.Any(), params.OfferArg{
			OfferUUID:     s.offerUUID,
			Macaroons:     macaroon.Slice{s.macaroon},
			BakeryVersion: bakery.LatestVersion,
		}).
		DoAndReturn(func(ctx context.Context, oa params.OfferArg) (watcher.OfferStatusWatcher, error) {
			defer close(done)

			ch := make(chan []watcher.OfferStatusChange)
			return watchertest.NewMockWatcher(ch), nil
		})

	w, err := NewRemoteApplicationWorker(s.newApplicationConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	workertest.CleanKill(c, w)
}

func (s *applicationWorkerSuite) TestStartFailedWatchApplicationLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {
			defer close(done)
			return nil, applicationerrors.ApplicationNotFound
		})

	w, err := NewRemoteApplicationWorker(s.newApplicationConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationWorkerSuite) TestStartNoRemoteClient(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {
			ch := make(chan []string)
			return watchertest.NewMockStringsWatcher(ch), nil
		})

	s.remoteRelationClientGetter.EXPECT().
		GetRemoteRelationClient(gomock.Any(), s.remoteModelUUID).
		Return(s.remoteModelRelationClient, errors.NotFound)

	s.crossModelService.EXPECT().
		SetRemoteApplicationOffererStatus(gomock.Any(), s.applicationUUID, status.StatusInfo{
			Status:  status.Error,
			Message: "cannot connect to external controller: not found",
		}).
		DoAndReturn(func(ctx context.Context, i application.UUID, si status.StatusInfo) error {
			defer close(done)
			return nil
		})

	w, err := NewRemoteApplicationWorker(s.newApplicationConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, `cannot connect to external controller: not found`)
}

func (s *applicationWorkerSuite) TestStartWatchOfferStatusFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {
			ch := make(chan []string)
			return watchertest.NewMockStringsWatcher(ch), nil
		})

	s.remoteRelationClientGetter.EXPECT().
		GetRemoteRelationClient(gomock.Any(), s.remoteModelUUID).
		Return(s.remoteModelRelationClient, nil)

	s.remoteModelRelationClient.EXPECT().
		WatchOfferStatus(gomock.Any(), params.OfferArg{
			OfferUUID:     s.offerUUID,
			Macaroons:     macaroon.Slice{s.macaroon},
			BakeryVersion: bakery.LatestVersion,
		}).
		DoAndReturn(func(ctx context.Context, oa params.OfferArg) (watcher.OfferStatusWatcher, error) {
			defer close(done)
			return nil, errors.NotValid
		})

	w, err := NewRemoteApplicationWorker(s.newApplicationConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, `watching status for offer: not valid`)
}

func (s *applicationWorkerSuite) newApplicationConfig(c *tc.C) RemoteApplicationConfig {
	return RemoteApplicationConfig{
		CrossModelService:          s.crossModelService,
		RemoteRelationClientGetter: s.remoteRelationClientGetter,
		OfferUUID:                  s.offerUUID,
		ApplicationName:            "foo",
		ApplicationUUID:            s.applicationUUID,
		LocalModelUUID:             tc.Must(c, model.NewUUID),
		RemoteModelUUID:            s.remoteModelUUID,
		ConsumeVersion:             1,
		Macaroon:                   s.macaroon,
		NewLocalUnitRelationsWorker: func(c localunitrelations.Config) (localunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewRemoteUnitRelationsWorker: func(c remoteunitrelations.Config) (remoteunitrelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		NewRemoteRelationsWorker: func(c remoterelations.Config) (remoterelations.ReportableWorker, error) {
			return newErrWorker(nil), nil
		},
		Clock:  clock.WallClock,
		Logger: s.logger,
	}
}
