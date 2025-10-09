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
	gomock "go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	domaincharm "github.com/juju/juju/domain/application/charm"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	crossModelRelationService *MockCrossModelRelationService
	crossModelAuthContext     *MockCrossModelAuthContext
	authenticator             *MockMacaroonAuthenticator

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
	offerUUID := tc.Must(c, uuid.NewUUID).String()
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
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID, gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	relKey, err := corerelation.NewKeyFromString("offerapp:db remoteapp:db")
	c.Assert(err, tc.ErrorIsNil)
	offererRemoteRelationTag := names.NewRelationTag(relKey.String())

	s.crossModelAuthContext.EXPECT().CreateRemoteRelationMacaroon(gomock.Any(), s.modelUUID, offerUUID, "bob", offererRemoteRelationTag, bakery.LatestVersion).
		Return(&bakery.Macaroon{}, nil)

	api := s.api(c)
	arg := s.relationArg(remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", macaroon.Slice{testMac})
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

	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return("", application.UUID(""), errors.New("boom"))

	api := s.api(c)
	arg := s.relationArg(remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "boom")
}

func (s *facadeSuite) TestRegisterRemoteRelationsAuthError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID, gomock.Any(), bakery.LatestVersion).
		Return(nil, errors.New("boom"))

	api := s.api(c)
	arg := s.relationArg(remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "boom")
}

func (s *facadeSuite) TestRegisterRemoteRelationsMissingUsername(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID, gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{}, nil)

	api := s.api(c)
	arg := s.relationArg(remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, apiservererrors.ErrPerm.Error())
}

func (s *facadeSuite) TestRegisterRemoteRelationsAddConsumerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)
	s.crossModelRelationService.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), gomock.Any()).
		Return(errors.New("insert failed"))

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID, gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	api := s.api(c)
	arg := s.relationArg(remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "adding remote application consumer: insert failed")
}

func (s *facadeSuite) TestRegisterRemoteRelationsRelationKeyParseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelRelationService.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), gomock.Any()).
		Return(nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID, gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	api := s.api(c)
	// remote endpoint name lacks application prefix -> parse failure.
	arg := s.relationArg(remoteAppToken, offerUUID, relationUUID, "db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "parsing relation key.*")
}

func (s *facadeSuite) TestRegisterRemoteRelationsCreateMacaroonError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "offerapp"
	appUUIDStr := tc.Must(c, uuid.NewUUID).String()
	offerUUID := tc.Must(c, uuid.NewUUID).String()
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	remoteAppToken := tc.Must(c, uuid.NewUUID).String()

	s.crossModelRelationService.EXPECT().
		GetApplicationNameAndUUIDByOfferUUID(gomock.Any(), offerUUID).
		Return(appName, application.UUID(appUUIDStr), nil)

	s.crossModelRelationService.EXPECT().
		AddRemoteApplicationConsumer(gomock.Any(), gomock.Any()).
		Return(nil)

	s.crossModelAuthContext.EXPECT().Authenticator().Return(s.authenticator)
	s.authenticator.EXPECT().CheckOfferMacaroons(gomock.Any(), s.modelUUID.String(), offerUUID, gomock.Any(), bakery.LatestVersion).
		Return(map[string]string{"username": "bob"}, nil)

	s.crossModelAuthContext.EXPECT().CreateRemoteRelationMacaroon(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("mint failed"))

	api := s.api(c)
	arg := s.relationArg(remoteAppToken, offerUUID, relationUUID, "remoteapp:db", "db", nil)
	results, err := api.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArgs{Relations: []params.RegisterRemoteRelationArg{arg}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "creating relation macaroon: mint failed")
}

func (s *facadeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.crossModelRelationService = NewMockCrossModelRelationService(ctrl)
	s.crossModelAuthContext = NewMockCrossModelAuthContext(ctrl)
	s.authenticator = NewMockMacaroonAuthenticator(ctrl)
	c.Cleanup(func() {
		s.crossModelRelationService = nil
		s.crossModelAuthContext = nil
		s.authenticator = nil
	})
	return ctrl
}

func (s *facadeSuite) api(c *tc.C) *CrossModelRelationsAPIv3 {
	api, err := NewCrossModelRelationsAPI(
		s.modelUUID,
		s.crossModelAuthContext,
		s.crossModelRelationService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *facadeSuite) relationArg(appToken, offerUUID, relationUUID, remoteEndpoint, localEndpoint string, macs macaroon.Slice) params.RegisterRemoteRelationArg {
	return params.RegisterRemoteRelationArg{
		ApplicationToken:  appToken,
		OfferUUID:         offerUUID,
		RelationToken:     relationUUID,
		RemoteEndpoint:    params.RemoteEndpoint{Name: remoteEndpoint, Role: internalcharm.RoleProvider, Interface: "database"},
		LocalEndpointName: localEndpoint,
		Macaroons:         macs,
		BakeryVersion:     bakery.LatestVersion,
		ConsumeVersion:    1,
		SourceModelTag:    "",
	}
}
