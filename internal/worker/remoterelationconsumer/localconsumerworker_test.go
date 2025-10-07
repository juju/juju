// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	stdtesting "testing"
	"time"

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
	"github.com/juju/juju/internal/worker/remoterelationconsumer/consumerunitrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/offererrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/offererunitrelations"
	"github.com/juju/juju/rpc/params"
)

func TestLocalConsumerWorker(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &localConsumerWorkerSuite{})
}

type localConsumerWorkerSuite struct {
	baseSuite

	applicationName   string
	applicationUUID   application.UUID
	offererModelUUID  string
	consumerModelUUID model.UUID
	offerUUID         string
	macaroon          *macaroon.Macaroon

	consumerUnitRelationsWorkerStarted chan struct{}
	offererUnitRelationsWorkerStarted  chan struct{}
	offererRelationWorkerStarted       chan struct{}
}

func (s *localConsumerWorkerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.applicationName = "foo"
	s.applicationUUID = tc.Must(c, application.NewID)
	s.offererModelUUID = tc.Must(c, model.NewUUID).String()
	s.consumerModelUUID = tc.Must(c, model.NewUUID)
	s.offerUUID = tc.Must(c, uuid.NewUUID).String()

	s.macaroon = newMacaroon(c, "test")
}

func (s *localConsumerWorkerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newLocalConsumerWorkerConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.CrossModelService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.RemoteRelationClientGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.OfferUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.ApplicationName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.ApplicationUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.ConsumerModelUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.OffererModelUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.Macaroon = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.NewConsumerUnitRelationsWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.NewOffererUnitRelationsWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.NewOffererRelationsWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newLocalConsumerWorkerConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *localConsumerWorkerSuite) TestStart(c *tc.C) {
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

	w, err := NewLocalConsumerWorker(s.newLocalConsumerWorkerConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchOfferStatus to be called")
	}

	workertest.CleanKill(c, w)
}

func (s *localConsumerWorkerSuite) TestStartFailedWatchApplicationLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})

	s.crossModelService.EXPECT().
		WatchApplicationLifeSuspendedStatus(gomock.Any(), s.applicationUUID).
		DoAndReturn(func(ctx context.Context, i application.UUID) (watcher.StringsWatcher, error) {
			defer close(done)
			return nil, applicationerrors.ApplicationNotFound
		})

	w, err := NewLocalConsumerWorker(s.newLocalConsumerWorkerConfig(c))
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

func (s *localConsumerWorkerSuite) TestStartNoRemoteClient(c *tc.C) {
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

	w, err := NewLocalConsumerWorker(s.newLocalConsumerWorkerConfig(c))
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

func (s *localConsumerWorkerSuite) TestStartWatchOfferStatusFailed(c *tc.C) {
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

	w, err := NewLocalConsumerWorker(s.newLocalConsumerWorkerConfig(c))
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

func (s *localConsumerWorkerSuite) TestWatchApplicationStatusChanged(c *tc.C) {
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

	w, err := NewLocalConsumerWorker(s.newLocalConsumerWorkerConfig(c))
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

func (s *localConsumerWorkerSuite) TestWatchApplicationStatusChangedNotFound(c *tc.C) {
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

	w, err := NewLocalConsumerWorker(s.newLocalConsumerWorkerConfig(c))
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

func (s *localConsumerWorkerSuite) TestWatchApplicationStatusChangedError(c *tc.C) {
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

	w, err := NewLocalConsumerWorker(s.newLocalConsumerWorkerConfig(c))
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

func (s *localConsumerWorkerSuite) TestHandleRelationChange(c *tc.C) {
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

	w := s.newLocalConsumerWorker(c)
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

func (s *localConsumerWorkerSuite) TestHandleRelationChangeOneEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	w := s.newLocalConsumerWorker(c)
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

func (s *localConsumerWorkerSuite) TestHandleRelationChangeNoMatchingEndpointApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	w := s.newLocalConsumerWorker(c)
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

func (s *localConsumerWorkerSuite) TestHandleRelationChangePeerRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	done := s.expectWorkerStartup(c)

	w := s.newLocalConsumerWorker(c)
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

func (s *localConsumerWorkerSuite) TestRegisterConsumerRelation(c *tc.C) {
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

	w := s.newLocalConsumerWorker(c)
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
		offererApplicationUUID: token,
		macaroon:               mac,
	})
}

func (s *localConsumerWorkerSuite) TestRegisterConsumerRelationFailedRequest(c *tc.C) {
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

	w := s.newLocalConsumerWorker(c)
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

func (s *localConsumerWorkerSuite) TestRegisterConsumerRelationInvalidResultLength(c *tc.C) {
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

	w := s.newLocalConsumerWorker(c)
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
	c.Assert(err, tc.ErrorMatches, `.*no result from registering consumer relation.*`)
}

func (s *localConsumerWorkerSuite) TestRegisterConsumerRelationFailedRequestError(c *tc.C) {
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

	w := s.newLocalConsumerWorker(c)
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

func (s *localConsumerWorkerSuite) TestRegisterConsumerRelationFailedToSaveMacaroon(c *tc.C) {
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

	w := s.newLocalConsumerWorker(c)
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

func (s *localConsumerWorkerSuite) TestHandleRelationConsumption(c *tc.C) {
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

	w := s.newLocalConsumerWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	err := w.handleRelationConsumption(c.Context(), domainrelation.RelationDetails{
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
		Suspended: false,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.waitForAllWorkersStarted(c)

	// Ensure that we create remote relation worker.
	names := w.runner.WorkerNames()
	c.Assert(names, tc.HasLen, 3)
	c.Check(names, tc.SameContents, []string{
		"offerer-relation:" + relationUUID.String(),
		"offerer-unit-relation:" + relationUUID.String(),
		"consumer-unit-relation:" + relationUUID.String(),
	})

	workertest.CleanKill(c, w)
}

func (s *localConsumerWorkerSuite) TestHandleRelationConsumptionEnsureSingular(c *tc.C) {
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
		}}, nil).Times(2)
	s.crossModelService.EXPECT().
		SaveMacaroonForRelation(gomock.Any(), relationUUID, mac).
		Return(nil).Times(2)

	w := s.newLocalConsumerWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to be started")
	}

	err := w.handleRelationConsumption(c.Context(), domainrelation.RelationDetails{
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
		Suspended: false,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.waitForAllWorkersStarted(c)

	// Ensure that we create remote relation worker.
	names := w.runner.WorkerNames()
	c.Assert(names, tc.HasLen, 3)
	c.Check(names, tc.SameContents, []string{
		"offerer-relation:" + relationUUID.String(),
		"offerer-unit-relation:" + relationUUID.String(),
		"consumer-unit-relation:" + relationUUID.String(),
	})

	err = w.handleRelationConsumption(c.Context(), domainrelation.RelationDetails{
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
		Suspended: false,
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-s.offererRelationWorkerStarted:
		c.Fatalf("remote relation worker started more than once")
	case <-time.After(500 * time.Millisecond):
		// Wait for a bit to ensure we don't get a second start.
	}

	// Ensure that calling handleRelationConsumption again doesn't create
	// another remote relation worker.
	names = w.runner.WorkerNames()
	c.Assert(names, tc.HasLen, 3)
	c.Check(names, tc.SameContents, []string{
		"offerer-relation:" + relationUUID.String(),
		"offerer-unit-relation:" + relationUUID.String(),
		"consumer-unit-relation:" + relationUUID.String(),
	})

	workertest.CleanKill(c, w)
}

func (s *localConsumerWorkerSuite) newLocalConsumerWorker(c *tc.C) *localConsumerWorker {
	return tc.Must1(c, NewLocalConsumerWorker, s.newLocalConsumerWorkerConfig(c)).(*localConsumerWorker)
}

func (s *localConsumerWorkerSuite) newLocalConsumerWorkerConfig(c *tc.C) LocalConsumerWorkerConfig {
	return LocalConsumerWorkerConfig{
		CrossModelService:          s.crossModelService,
		RemoteRelationClientGetter: s.remoteRelationClientGetter,
		OfferUUID:                  s.offerUUID,
		ApplicationName:            s.applicationName,
		ApplicationUUID:            s.applicationUUID,
		ConsumerModelUUID:          s.consumerModelUUID,
		OffererModelUUID:           s.offererModelUUID,
		ConsumeVersion:             1,
		Macaroon:                   s.macaroon,
		NewConsumerUnitRelationsWorker: func(consumerunitrelations.Config) (consumerunitrelations.ReportableWorker, error) {
			defer func() {
				select {
				case s.consumerUnitRelationsWorkerStarted <- struct{}{}:
				case <-c.Context().Done():
					c.Fatalf("timed out trying to send on offererRelationWorkerStarted channel")
				}
			}()
			return newErrWorker(nil), nil
		},
		NewOffererUnitRelationsWorker: func(offererunitrelations.Config) (offererunitrelations.ReportableWorker, error) {
			defer func() {
				select {
				case s.offererUnitRelationsWorkerStarted <- struct{}{}:
				case <-c.Context().Done():
					c.Fatalf("timed out trying to send on offererUnitRelationsWorkerStarted channel")
				}
			}()
			return newErrWorker(nil), nil
		},
		NewOffererRelationsWorker: func(offererrelations.Config) (offererrelations.ReportableWorker, error) {
			defer func() {
				select {
				case s.offererRelationWorkerStarted <- struct{}{}:
				case <-c.Context().Done():
					c.Fatalf("timed out trying to send on offererRelationWorkerStarted channel")
				}
			}()
			return newErrWorker(nil), nil
		},
		Clock:  clock.WallClock,
		Logger: s.logger,
	}
}

func (s *localConsumerWorkerSuite) expectWorkerStartup(c *tc.C) <-chan struct{} {
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

func (s *localConsumerWorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.offererRelationWorkerStarted = make(chan struct{}, 1)
	s.offererUnitRelationsWorkerStarted = make(chan struct{}, 1)
	s.consumerUnitRelationsWorkerStarted = make(chan struct{}, 1)

	return ctrl
}

func (s *localConsumerWorkerSuite) waitForAllWorkersStarted(c *tc.C) {
	select {
	case <-s.consumerUnitRelationsWorkerStarted:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for remote relation worker to be started")
	}

	select {
	case <-s.offererUnitRelationsWorkerStarted:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for remote relation worker to be started")
	}

	select {
	case <-s.offererRelationWorkerStarted:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for remote relation worker to be started")
	}
}
