// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
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

	applicationName   string
	applicationUUID   application.UUID
	offererModelUUID  string
	consumerModelUUID model.UUID
	offerUUID         string
	macaroon          *macaroon.Macaroon
}

func (s *applicationWorkerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationName = "foo"
	s.applicationUUID = tc.Must(c, application.NewID)
	s.offererModelUUID = tc.Must(c, model.NewUUID).String()
	s.consumerModelUUID = tc.Must(c, model.NewUUID)
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
		GetRemoteRelationClient(gomock.Any(), s.offererModelUUID).
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
		GetRemoteRelationClient(gomock.Any(), s.offererModelUUID).
		Return(s.remoteModelRelationClient, errors.NotFound)

	s.crossModelService.EXPECT().
		SetRemoteApplicationOffererStatus(gomock.Any(), s.applicationName, status.StatusInfo{
			Status:  status.Error,
			Message: "cannot connect to external controller: not found",
		}).
		DoAndReturn(func(context.Context, string, status.StatusInfo) error {
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
		GetRemoteRelationClient(gomock.Any(), s.offererModelUUID).
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

func (s *applicationWorkerSuite) TestWatchApplicationStatusChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := make(chan struct{})

	ch := make(chan []string)
	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {

			return watchertest.NewMockStringsWatcher(ch), nil
		})
	s.remoteRelationClientGetter.EXPECT().
		GetRemoteRelationClient(gomock.Any(), s.offererModelUUID).
		Return(s.remoteModelRelationClient, nil)

	s.remoteModelRelationClient.EXPECT().
		WatchOfferStatus(gomock.Any(), params.OfferArg{
			OfferUUID:     s.offerUUID,
			Macaroons:     macaroon.Slice{s.macaroon},
			BakeryVersion: bakery.LatestVersion,
		}).
		DoAndReturn(func(ctx context.Context, oa params.OfferArg) (watcher.OfferStatusWatcher, error) {
			ch := make(chan []watcher.OfferStatusChange)
			return watchertest.NewMockWatcher(ch), nil
		})

	// Handle the change.
	s.crossModelService.EXPECT().
		GetRelationDetails(gomock.Any(), relationUUID).
		DoAndReturn(func(ctx context.Context, u relation.UUID) (domainrelation.RelationDetails, error) {
			defer close(done)

			return domainrelation.RelationDetails{
				UUID: relationUUID,
				Life: life.Alive,
			}, nil
		})

	w, err := NewRemoteApplicationWorker(s.newApplicationConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{relationUUID.String()}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send on application status channel")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	// We don't want to test the full loop, just that we handle the change.
	// The rest of the logic is covered in other tests.

	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *applicationWorkerSuite) TestWatchApplicationStatusChangedNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := make(chan struct{})

	ch := make(chan []string)
	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {

			return watchertest.NewMockStringsWatcher(ch), nil
		})
	s.remoteRelationClientGetter.EXPECT().
		GetRemoteRelationClient(gomock.Any(), s.offererModelUUID).
		Return(s.remoteModelRelationClient, nil)

	s.remoteModelRelationClient.EXPECT().
		WatchOfferStatus(gomock.Any(), params.OfferArg{
			OfferUUID:     s.offerUUID,
			Macaroons:     macaroon.Slice{s.macaroon},
			BakeryVersion: bakery.LatestVersion,
		}).
		DoAndReturn(func(ctx context.Context, oa params.OfferArg) (watcher.OfferStatusWatcher, error) {
			ch := make(chan []watcher.OfferStatusChange)
			return watchertest.NewMockWatcher(ch), nil
		})

	// Handle the change.
	s.crossModelService.EXPECT().
		GetRelationDetails(gomock.Any(), relationUUID).
		DoAndReturn(func(ctx context.Context, u relation.UUID) (domainrelation.RelationDetails, error) {
			defer close(done)

			return domainrelation.RelationDetails{}, relationerrors.RelationNotFound
		})

	w, err := NewRemoteApplicationWorker(s.newApplicationConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{relationUUID.String()}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send on application status channel")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	// We don't want to test the full loop, just that we handle the change.
	// The rest of the logic is covered in other tests.

	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *applicationWorkerSuite) TestWatchApplicationStatusChangedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := make(chan struct{})

	ch := make(chan []string)
	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(ch), nil
		})
	s.remoteRelationClientGetter.EXPECT().
		GetRemoteRelationClient(gomock.Any(), s.offererModelUUID).
		Return(s.remoteModelRelationClient, nil)

	s.remoteModelRelationClient.EXPECT().
		WatchOfferStatus(gomock.Any(), params.OfferArg{
			OfferUUID:     s.offerUUID,
			Macaroons:     macaroon.Slice{s.macaroon},
			BakeryVersion: bakery.LatestVersion,
		}).
		DoAndReturn(func(ctx context.Context, oa params.OfferArg) (watcher.OfferStatusWatcher, error) {
			ch := make(chan []watcher.OfferStatusChange)
			return watchertest.NewMockWatcher(ch), nil
		})

	// Handle the change.
	s.crossModelService.EXPECT().
		GetRelationDetails(gomock.Any(), relationUUID).
		DoAndReturn(func(ctx context.Context, u relation.UUID) (domainrelation.RelationDetails, error) {
			defer close(done)

			return domainrelation.RelationDetails{}, internalerrors.Errorf("front fell off")
		})

	w, err := NewRemoteApplicationWorker(s.newApplicationConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- []string{relationUUID.String()}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send on application status channel")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	// We don't want to test the full loop, just that we handle the change.
	// The rest of the logic is covered in other tests.

	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorMatches, `.*front fell off.*`)
}

func (s *applicationWorkerSuite) TestHandleRelationChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)
	mac := newMacaroon(c, "test")

	done := s.expectWorkerStartup(c)

	arg := params.RegisterRemoteRelationArg{
		ApplicationToken: s.applicationUUID.String(),
		SourceModelTag:   names.NewModelTag(s.consumerModelUUID.String()).String(),
		RelationToken:    relationUUID.String(),
		OfferUUID:        s.offerUUID,
		Macaroons:        macaroon.Slice{s.macaroon},
		RemoteEndpoint: params.RemoteEndpoint{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
		},
		LocalEndpointName: "blog",
		ConsumeVersion:    1,
		BakeryVersion:     bakery.LatestVersion,
	}

	s.remoteModelRelationClient.EXPECT().
		RegisterRemoteRelations(gomock.Any(), arg).
		Return([]params.RegisterRemoteRelationResult{{
			Result: &params.RemoteRelationDetails{
				Token:         tc.Must(c, uuid.NewUUID).String(),
				Macaroon:      mac,
				BakeryVersion: bakery.LatestVersion,
			},
		}}, nil)
	s.crossModelService.EXPECT().
		SaveMacaroonForRelation(gomock.Any(), relationUUID, mac).
		Return(nil)

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	err := w.handleRelationChange(c.Context(), domainrelation.RelationDetails{
		UUID: relationUUID,
		Life: life.Alive,
		Endpoints: []domainrelation.Endpoint{{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		}, {
			ApplicationName: "bar",
			Relation: charm.Relation{
				Name:      "blog",
				Role:      charm.RoleRequirer,
				Interface: "blog",
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationWorkerSuite) TestHandleRelationChangeOneEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	err := w.handleRelationChange(c.Context(), domainrelation.RelationDetails{
		UUID: relationUUID,
		Life: life.Alive,
		Endpoints: []domainrelation.Endpoint{{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		}},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *applicationWorkerSuite) TestHandleRelationChangeNoMatchingEndpointApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	err := w.handleRelationChange(c.Context(), domainrelation.RelationDetails{
		UUID: relationUUID,
		Life: life.Alive,
		Endpoints: []domainrelation.Endpoint{{
			ApplicationName: "bar",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		}, {
			ApplicationName: "bar",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		}},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *applicationWorkerSuite) TestHandleRelationChangePeerRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	err := w.handleRelationChange(c.Context(), domainrelation.RelationDetails{
		UUID: relationUUID,
		Life: life.Alive,
		Endpoints: []domainrelation.Endpoint{{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		}, {
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		}},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *applicationWorkerSuite) TestRegisterConsumerRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	token := tc.Must(c, application.NewID)
	relationUUID := tc.Must(c, relation.NewUUID)
	mac := newMacaroon(c, "test")

	done := s.expectWorkerStartup(c)

	arg := params.RegisterRemoteRelationArg{
		ApplicationToken: s.applicationUUID.String(),
		SourceModelTag:   names.NewModelTag(s.consumerModelUUID.String()).String(),
		RelationToken:    relationUUID.String(),
		OfferUUID:        s.offerUUID,
		Macaroons:        macaroon.Slice{s.macaroon},
		RemoteEndpoint: params.RemoteEndpoint{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
		},
		LocalEndpointName: "blog",
		ConsumeVersion:    1,
		BakeryVersion:     bakery.LatestVersion,
	}

	s.remoteModelRelationClient.EXPECT().
		RegisterRemoteRelations(gomock.Any(), arg).
		Return([]params.RegisterRemoteRelationResult{{
			Result: &params.RemoteRelationDetails{
				Token:         token.String(),
				Macaroon:      mac,
				BakeryVersion: bakery.LatestVersion,
			},
		}}, nil)
	s.crossModelService.EXPECT().
		SaveMacaroonForRelation(gomock.Any(), relationUUID, mac).
		Return(nil)

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	result, err := w.registerConsumerRelation(c.Context(),
		relationUUID,
		s.offerUUID,
		1,
		domainrelation.Endpoint{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		},
		"blog",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, consumerRelationResult{
		offeringApplicationUUID: token,
		macaroon:                mac,
	})
}

func (s *applicationWorkerSuite) TestRegisterConsumerRelationFailedRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	arg := params.RegisterRemoteRelationArg{
		ApplicationToken: s.applicationUUID.String(),
		SourceModelTag:   names.NewModelTag(s.consumerModelUUID.String()).String(),
		RelationToken:    relationUUID.String(),
		OfferUUID:        s.offerUUID,
		Macaroons:        macaroon.Slice{s.macaroon},
		RemoteEndpoint: params.RemoteEndpoint{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
		},
		LocalEndpointName: "blog",
		ConsumeVersion:    1,
		BakeryVersion:     bakery.LatestVersion,
	}

	s.remoteModelRelationClient.EXPECT().
		RegisterRemoteRelations(gomock.Any(), arg).
		Return(nil, internalerrors.New("front fell off"))

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	_, err := w.registerConsumerRelation(c.Context(),
		relationUUID,
		s.offerUUID,
		1,
		domainrelation.Endpoint{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		},
		"blog",
	)
	c.Assert(err, tc.ErrorMatches, `.*front fell off.*`)
}

func (s *applicationWorkerSuite) TestRegisterConsumerRelationInvalidResultLength(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	arg := params.RegisterRemoteRelationArg{
		ApplicationToken: s.applicationUUID.String(),
		SourceModelTag:   names.NewModelTag(s.consumerModelUUID.String()).String(),
		RelationToken:    relationUUID.String(),
		OfferUUID:        s.offerUUID,
		Macaroons:        macaroon.Slice{s.macaroon},
		RemoteEndpoint: params.RemoteEndpoint{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
		},
		LocalEndpointName: "blog",
		ConsumeVersion:    1,
		BakeryVersion:     bakery.LatestVersion,
	}

	s.remoteModelRelationClient.EXPECT().
		RegisterRemoteRelations(gomock.Any(), arg).
		Return([]params.RegisterRemoteRelationResult{}, nil)

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	_, err := w.registerConsumerRelation(c.Context(),
		relationUUID,
		s.offerUUID,
		1,
		domainrelation.Endpoint{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		},
		"blog",
	)
	c.Assert(err, tc.ErrorMatches, `.*no result from registering remote relation.*`)
}

func (s *applicationWorkerSuite) TestRegisterConsumerRelationFailedRequestError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	arg := params.RegisterRemoteRelationArg{
		ApplicationToken: s.applicationUUID.String(),
		SourceModelTag:   names.NewModelTag(s.consumerModelUUID.String()).String(),
		RelationToken:    relationUUID.String(),
		OfferUUID:        s.offerUUID,
		Macaroons:        macaroon.Slice{s.macaroon},
		RemoteEndpoint: params.RemoteEndpoint{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
		},
		LocalEndpointName: "blog",
		ConsumeVersion:    1,
		BakeryVersion:     bakery.LatestVersion,
	}

	s.remoteModelRelationClient.EXPECT().
		RegisterRemoteRelations(gomock.Any(), arg).
		Return([]params.RegisterRemoteRelationResult{{
			Error: &params.Error{
				Code:    params.CodeBadRequest,
				Message: "bad request",
			},
		}}, nil)

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	_, err := w.registerConsumerRelation(c.Context(),
		relationUUID,
		s.offerUUID,
		1,
		domainrelation.Endpoint{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		},
		"blog",
	)
	c.Assert(err, tc.ErrorMatches, `.*registering relation.*`)
}

func (s *applicationWorkerSuite) TestRegisterConsumerRelationFailedToSaveMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	token := tc.Must(c, application.NewID)
	relationUUID := tc.Must(c, relation.NewUUID)
	mac := newMacaroon(c, "test")

	done := s.expectWorkerStartup(c)

	arg := params.RegisterRemoteRelationArg{
		ApplicationToken: s.applicationUUID.String(),
		SourceModelTag:   names.NewModelTag(s.consumerModelUUID.String()).String(),
		RelationToken:    relationUUID.String(),
		OfferUUID:        s.offerUUID,
		Macaroons:        macaroon.Slice{s.macaroon},
		RemoteEndpoint: params.RemoteEndpoint{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "db",
		},
		LocalEndpointName: "blog",
		ConsumeVersion:    1,
		BakeryVersion:     bakery.LatestVersion,
	}

	s.remoteModelRelationClient.EXPECT().
		RegisterRemoteRelations(gomock.Any(), arg).
		Return([]params.RegisterRemoteRelationResult{{
			Result: &params.RemoteRelationDetails{
				Token:         token.String(),
				Macaroon:      mac,
				BakeryVersion: bakery.LatestVersion,
			},
		}}, nil)
	s.crossModelService.EXPECT().
		SaveMacaroonForRelation(gomock.Any(), relationUUID, mac).
		Return(internalerrors.Errorf("front fell off"))

	w := s.newRemoteRelationsWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	_, err := w.registerConsumerRelation(c.Context(),
		relationUUID,
		s.offerUUID,
		1,
		domainrelation.Endpoint{
			ApplicationName: "foo",
			Relation: charm.Relation{
				Name:      "db",
				Role:      charm.RoleProvider,
				Interface: "db",
			},
		},
		"blog",
	)
	c.Assert(err, tc.ErrorMatches, `.*front fell off.*`)
}

func (s *applicationWorkerSuite) newRemoteRelationsWorker(c *tc.C) *remoteApplicationWorker {
	return tc.Must1(c, NewRemoteApplicationWorker, s.newApplicationConfig(c)).(*remoteApplicationWorker)
}

func (s *applicationWorkerSuite) newApplicationConfig(c *tc.C) RemoteApplicationConfig {
	return RemoteApplicationConfig{
		CrossModelService:          s.crossModelService,
		RemoteRelationClientGetter: s.remoteRelationClientGetter,
		OfferUUID:                  s.offerUUID,
		ApplicationName:            s.applicationName,
		ApplicationUUID:            s.applicationUUID,
		LocalModelUUID:             s.consumerModelUUID,
		RemoteModelUUID:            s.offererModelUUID,
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

func (s *applicationWorkerSuite) expectWorkerStartup(c *tc.C) <-chan struct{} {
	done := make(chan struct{})

	ch := make(chan []string)
	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(ch), nil
		})
	s.remoteRelationClientGetter.EXPECT().
		GetRemoteRelationClient(gomock.Any(), s.offererModelUUID).
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

	return done
}
