// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"errors"
	"testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	domaincharm "github.com/juju/juju/domain/application/charm"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	testhelpers.IsolationSuite

	watcherRegistry *facademocks.MockWatcherRegistry

	applicationService        *MockApplicationService
	crossModelRelationService *MockCrossModelRelationService
	relationService           *MockRelationService
	removalService            *MockRemovalService
	secretService             *MockSecretService
	statusService             *MockStatusService

	crossModelAuthContext *MockCrossModelAuthContext
	authenticator         *MockMacaroonAuthenticator

	modelUUID model.UUID
}

func TestFacadeSuite(t *testing.T) {
	tc.Run(t, &facadeSuite{})
}

// SetUpTest runs before each test in the suite.
func (s *facadeSuite) SetUpTest(c *tc.C) {
	s.modelUUID = model.UUID(tc.Must(c, uuid.NewUUID).String())
}

func (s *facadeSuite) TestRegisterRemoteRelationsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()
	testMac, err := macaroon.New([]byte("root"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	var received crossmodelrelationservice.AddRemoteApplicationConsumerArgs
	s.crossModelRelationService.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, args crossmodelrelationservice.AddRemoteApplicationConsumerArgs) error {
			received = args
			return nil
		})

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	relKey, err := corerelation.NewKeyFromString("offerapp:db remoteapp:db")
	c.Assert(err, tc.ErrorIsNil)
	offererRemoteRelationTag := names.NewRelationTag(relKey.String())

	s.crossModelAuthContext.EXPECT().CreateRemoteRelationMacaroon(gomock.Any(), s.modelUUID, offerUUID.String(), "bob", offererRemoteRelationTag, bakery.LatestVersion).
		Return(&bakery.Macaroon{}, nil)

	api := s.api(c)
	arg := s.relationArg(c, remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", macaroon.Slice{testMac})
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{
		Relations: []params.RegisterRemoteRelationArg{arg},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Check(results.Results[0].Result.Token, tc.Equals, appUUIDStr)

	c.Check(received.OfferUUID, tc.Equals, offerUUID)
	c.Check(received.RelationUUID, tc.Equals, relationUUID)
	c.Check(received.RemoteApplicationUUID, tc.Equals, remoteAppToken)
	c.Check(received.Endpoints, tc.HasLen, 1)
	c.Check(received.Endpoints[0].Name, tc.Equals, "remoteapp:db")
	c.Check(received.Endpoints[0].Role, tc.Equals, domaincharm.RelationRole(internalcharm.RoleProvider))
}

func (s *facadeSuite) TestRegisterRemoteRelationsGetApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return("", application.UUID(""), errors.New("boom"))

	api := s.api(c)
	arg := s.relationArg(c, remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "boom")
}

func (s *facadeSuite) TestRegisterRemoteRelationsAuthError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(nil, errors.New("boom"))

	api := s.api(c)
	arg := s.relationArg(c, remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "boom")
}

func (s *facadeSuite) TestRegisterRemoteRelationsMissingUsername(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{}, nil)

	api := s.api(c)
	arg := s.relationArg(c, remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, apiservererrors.ErrPerm.Error())
}

func (s *facadeSuite) TestRegisterRemoteRelationsAddConsumerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)
	s.crossModelRelationService.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), gomock.Any()).
		Return(errors.New("insert failed"))

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	api := s.api(c)
	arg := s.relationArg(c, remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "adding remote application consumer: insert failed")
}

func (s *facadeSuite) TestRegisterRemoteRelationsRelationKeyParseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelRelationService.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), gomock.Any()).
		Return(nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	api := s.api(c)
	// remote endpoint name lacks application prefix -> parse failure.
	arg := s.relationArg(c, remoteAppToken, offerUUID, relationUUID, "db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "parsing relation key.*")
}

func (s *facadeSuite) TestRegisterRemoteRelationsCreateMacaroonError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelRelationService.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), gomock.Any()).
		Return(nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	s.crossModelAuthContext.EXPECT().CreateRemoteRelationMacaroon(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("mint failed"))

	api := s.api(c)
	arg := s.relationArg(c, remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "creating relation macaroon: mint failed")
}

func (s *facadeSuite) TestWatchOfferStatusNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	testMac, err := macaroon.New([]byte("root"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	offerUUID := tc.Must(c, offer.NewUUID)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(nil, nil)

	s.statusService.EXPECT().WatchOfferStatus(gomock.Any(), offerUUID).Return(nil, crossmodelrelationerrors.OfferNotFound)

	results, err := s.api(c).WatchOfferStatus(c.Context(), params.OfferArgs{
		Args: []params.OfferArg{{
			OfferUUID:     offerUUID.String(),
			BakeryVersion: bakery.LatestVersion,
			Macaroons:     macaroon.Slice{testMac},
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *facadeSuite) TestWatchOfferStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	testMac, err := macaroon.New([]byte("root"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	offerUUID := tc.Must(c, offer.NewUUID)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(nil, nil)

	changes := s.expectWatchOfferStatus(ctrl, offerUUID)
	changes <- struct{}{}
	s.statusService.EXPECT().GetOfferStatus(gomock.Any(), offerUUID).Return(corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "message",
	}, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, w worker.Worker) (string, error) {
			offerWatcher, ok := w.(OfferWatcher)
			if c.Check(ok, tc.IsTrue) {
				c.Check(offerWatcher.OfferUUID(), tc.Equals, offerUUID)
			}
			return "1", nil
		})

	results, err := s.api(c).WatchOfferStatus(c.Context(), params.OfferArgs{
		Args: []params.OfferArg{{
			OfferUUID:     offerUUID.String(),
			BakeryVersion: bakery.LatestVersion,
			Macaroons:     macaroon.Slice{testMac},
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.IsNil)
	c.Check(results.Results[0].OfferStatusWatcherId, tc.Equals, "1")
	c.Assert(results.Results[0].Changes, tc.HasLen, 1)
	c.Check(results.Results[0].Changes[0].Status.Status, tc.Equals, corestatus.Active)
	c.Check(results.Results[0].Changes[0].Status.Info, tc.Equals, "message")
}

func (s *facadeSuite) TestWatchOfferStatusAuthError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	offerUUID := tc.Must(c, offer.NewUUID)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(nil, errors.New("boom"))

	results, err := s.api(c).WatchOfferStatus(c.Context(), params.OfferArgs{
		Args: []params.OfferArg{{
			OfferUUID:     offerUUID.String(),
			BakeryVersion: bakery.LatestVersion,
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "boom")
}

func (s *facadeSuite) TestWatchConsumedSecretsChanges(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	testMac, err := macaroon.New([]byte("root"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	offerUUID := tc.Must(c, offer.NewUUID)
	relUUID := relationtesting.GenRelationUUID(c)
	appUUID := applicationtesting.GenApplicationUUID(c)
	uri := coresecrets.NewURI()

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.crossModelRelationService.EXPECT().GetOfferUUIDByRelationUUID(gomock.Any(), relUUID).Return(offerUUID, nil)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(nil, nil)

	changes := s.expectWatchConsumedSecretsChanges(ctrl, appUUID)
	changes <- []string{uri.ID}
	s.secretService.EXPECT().GetLatestRevisions(gomock.Any(), []*coresecrets.URI{uri}).
		Return(map[string]int{uri.ID: 666}, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, w worker.Worker) (string, error) {
			_, ok := w.(watcher.StringsWatcher)
			c.Assert(ok, tc.IsTrue)
			return "1", nil
		})

	results, err := s.api(c).WatchConsumedSecretsChanges(c.Context(), params.WatchRemoteSecretChangesArgs{
		Args: []params.WatchRemoteSecretChangesArg{{
			ApplicationToken: appUUID.String(),
			RelationToken:    relUUID.String(),
			Macaroons:        macaroon.Slice{testMac},
			BakeryVersion:    bakery.LatestVersion,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.IsNil)
	c.Check(results.Results[0].WatcherId, tc.Equals, "1")
	c.Assert(results.Results[0].Changes, tc.HasLen, 1)
	c.Check(results.Results[0].Changes[0].URI, tc.Equals, uri.String())
	c.Check(results.Results[0].Changes[0].LatestRevision, tc.Equals, 666)
}

func (s *facadeSuite) TestWatchConsumedSecretsChangesNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	testMac, err := macaroon.New([]byte("root"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	relUUID := relationtesting.GenRelationUUID(c)
	appUUID := applicationtesting.GenApplicationUUID(c)

	s.crossModelRelationService.EXPECT().GetOfferUUIDByRelationUUID(gomock.Any(), relUUID).Return("", crossmodelrelationerrors.OfferNotFound)

	results, err := s.api(c).WatchConsumedSecretsChanges(c.Context(), params.WatchRemoteSecretChangesArgs{
		Args: []params.WatchRemoteSecretChangesArg{{
			ApplicationToken: appUUID.String(),
			RelationToken:    relUUID.String(),
			Macaroons:        macaroon.Slice{testMac},
			BakeryVersion:    bakery.LatestVersion,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *facadeSuite) TestWatchConsumedSecretsChangesAuthError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	testMac, err := macaroon.New([]byte("root"), []byte("id"), "loc", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	offerUUID := tc.Must(c, offer.NewUUID)
	relUUID := relationtesting.GenRelationUUID(c)
	appUUID := applicationtesting.GenApplicationUUID(c)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.crossModelRelationService.EXPECT().GetOfferUUIDByRelationUUID(gomock.Any(), relUUID).Return(offerUUID, nil)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID.String(), gomock.Any(), bakery.LatestVersion).
		Return(nil, errors.New("boom"))

	results, err := s.api(c).WatchConsumedSecretsChanges(c.Context(), params.WatchRemoteSecretChangesArgs{
		Args: []params.WatchRemoteSecretChangesArg{{
			ApplicationToken: appUUID.String(),
			RelationToken:    relUUID.String(),
			Macaroons:        macaroon.Slice{testMac},
			BakeryVersion:    bakery.LatestVersion,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "boom")
}

func (s *facadeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.crossModelRelationService = NewMockCrossModelRelationService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.secretService = NewMockSecretService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.crossModelAuthContext = NewMockCrossModelAuthContext(ctrl)
	s.authenticator = NewMockMacaroonAuthenticator(ctrl)

	c.Cleanup(func() {
		s.watcherRegistry = nil

		s.applicationService = nil
		s.crossModelRelationService = nil
		s.relationService = nil
		s.removalService = nil
		s.secretService = nil
		s.statusService = nil
		s.crossModelAuthContext = nil
		s.authenticator = nil
	})
	return ctrl
}

func (s *facadeSuite) api(c *tc.C) *CrossModelRelationsAPIv3 {
	api, err := NewCrossModelRelationsAPI(
		s.modelUUID,
		s.crossModelAuthContext,
		s.watcherRegistry,
		s.applicationService,
		s.relationService,
		s.crossModelRelationService,
		s.statusService,
		s.secretService,
		s.removalService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *facadeSuite) relationArg(c *tc.C, appToken string, offerUUID offer.UUID, relationUUID, remoteEndpoint, localEndpoint string, macs macaroon.Slice) params.RegisterRemoteRelationArg {
	sourceModelUUID := tc.Must(c, model.NewUUID).String()
	return params.RegisterRemoteRelationArg{
		ApplicationToken:  appToken,
		OfferUUID:         offerUUID.String(),
		RelationToken:     relationUUID,
		RemoteEndpoint:    params.RemoteEndpoint{Name: remoteEndpoint, Role: internalcharm.RoleProvider, Interface: "database"},
		LocalEndpointName: localEndpoint,
		Macaroons:         macs,
		BakeryVersion:     bakery.LatestVersion,
		ConsumeVersion:    1,
		SourceModelTag:    names.NewModelTag(sourceModelUUID).String(),
	}
}

func (s *facadeSuite) expectWatchOfferStatus(ctrl *gomock.Controller, offerUUID offer.UUID) chan struct{} {
	mockWatcher := NewMockNotifyWatcher(ctrl)
	changes := make(chan struct{}, 1)
	mockWatcher.EXPECT().Changes().Return(changes)
	s.statusService.EXPECT().WatchOfferStatus(gomock.Any(), offerUUID).Return(mockWatcher, nil)
	return changes
}

func (s *facadeSuite) expectWatchConsumedSecretsChanges(ctrl *gomock.Controller, appUUID application.UUID) chan []string {
	mockWatcher := NewMockStringsWatcher(ctrl)
	changes := make(chan []string, 1)
	mockWatcher.EXPECT().Changes().Return(changes)
	s.crossModelRelationService.EXPECT().WatchRemoteConsumedSecretsChanges(gomock.Any(), appUUID).Return(mockWatcher, nil)
	return changes
}
