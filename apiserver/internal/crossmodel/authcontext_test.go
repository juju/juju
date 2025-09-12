// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	crossmodelbakery "github.com/juju/juju/apiserver/internal/crossmodel/bakery"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type authSuite struct {
	clock         *MockClock
	accessService *MockAccessService
	keyPair       *bakery.KeyPair
	bakery        *MockOfferBakery

	controllerUUID string
	modelUUID      model.UUID
}

func TestAuthSuite(t *testing.T) {
	tc.Run(t, &authSuite{})
}

func (s *authSuite) TestOfferThirdPartyKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	c.Assert(authContext.OfferThirdPartyKey(), tc.Equals, s.keyPair)
}

func (s *authSuite) TestCheckOfferAccessCaveatNotOfferPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.bakery.EXPECT().ParseCaveat("other-caveat").Return(crossmodelbakery.OfferAccessDetails{}, checkers.ErrCaveatNotRecognized)

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), "other-caveat")
	c.Assert(err, tc.ErrorIs, checkers.ErrCaveatNotRecognized)
}

func (s *authSuite) TestCheckOfferAccessCaveat(c *tc.C) {
	defer s.setupMocks(c).Finish()

	details := crossmodelbakery.OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	}
	caveat := s.newAccessCaveat(s.modelUUID.String())
	permissionCaveat := "has-offer-permission " + caveat

	s.bakery.EXPECT().ParseCaveat(permissionCaveat).Return(details, nil)

	authContext := s.newAuthContext(c)
	result, err := authContext.CheckOfferAccessCaveat(c.Context(), permissionCaveat)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, details)
}

func (s *authSuite) TestCheckOfferAccessCaveatInvalidSourceModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	details := crossmodelbakery.OfferAccessDetails{
		SourceModelUUID: "blah",
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	}
	caveat := s.newAccessCaveat("blah")
	permissionCaveat := "has-offer-permission " + caveat

	s.bakery.EXPECT().ParseCaveat(permissionCaveat).Return(details, nil)

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), permissionCaveat)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCheckOfferAccessCaveatInvalidUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	details := crossmodelbakery.OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "!not-a-user",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	}
	caveat := s.newAccessCaveatWithUser("!not-a-user")
	permissionCaveat := "has-offer-permission " + caveat

	s.bakery.EXPECT().ParseCaveat(permissionCaveat).Return(details, nil)

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), permissionCaveat)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCheckOfferAccessCaveatInvalidPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	details := crossmodelbakery.OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "blah",
	}
	caveat := s.newAccessCaveatWithPermission("blah")
	permissionCaveat := "has-offer-permission " + caveat

	s.bakery.EXPECT().ParseCaveat(permissionCaveat).Return(details, nil)

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), permissionCaveat)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCheckLocalAccessRequestControllerSuperuserAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.bakery.EXPECT().GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "mediawiki:db mysql:server").Return(s.caveatWithRelation(now))

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.controllerUUID,
	}).Return(permission.SuperuserAccess, nil)

	authContext := s.newAuthContext(c)
	caveats, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caveats, tc.DeepEquals, s.caveatWithRelation(now))
}

func (s *authSuite) TestCheckLocalAccessRequestControllerErrorBecomesErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.controllerUUID,
	}).Return(permission.SuperuserAccess, errors.Errorf("naughty"))

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authSuite) TestCheckLocalAccessRequestModelAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.bakery.EXPECT().GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "mediawiki:db mysql:server").Return(s.caveatWithRelation(now))

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.controllerUUID,
	}).Return(permission.AdminAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        s.modelUUID.String(),
	}).Return(permission.AdminAccess, nil)

	authContext := s.newAuthContext(c)
	caveats, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caveats, tc.DeepEquals, s.caveatWithRelation(now))
}

func (s *authSuite) TestCheckLocalAccessRequestModelErrorBecomesErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.controllerUUID,
	}).Return(permission.AdminAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        s.modelUUID.String(),
	}).Return(permission.AdminAccess, errors.Errorf("naughty"))

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authSuite) TestCheckLocalAccessRequestOfferConsume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.bakery.EXPECT().GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "mediawiki:db mysql:server").Return(s.caveatWithRelation(now))

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.controllerUUID,
	}).Return(permission.AdminAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        s.modelUUID.String(),
	}).Return(permission.ReadAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Offer,
		Key:        "mysql-uuid",
	}).Return(permission.ConsumeAccess, nil)

	authContext := s.newAuthContext(c)
	caveats, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caveats, tc.DeepEquals, s.caveatWithRelation(now))
}

func (s *authSuite) TestCheckLocalAccessRequestOfferConsumeInvalidConsume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.controllerUUID,
	}).Return(permission.AdminAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        s.modelUUID.String(),
	}).Return(permission.ReadAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Offer,
		Key:        "mysql-uuid",
	}).Return(permission.NoAccess, nil)

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authSuite) TestCheckLocalAccessRequestOfferConsumeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.controllerUUID,
	}).Return(permission.AdminAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        s.modelUUID.String(),
	}).Return(permission.ReadAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Offer,
		Key:        "mysql-uuid",
	}).Return(permission.NoAccess, coreerrors.NotValid)

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCreateConsumeMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	expected := &bakery.Macaroon{}

	s.bakery.EXPECT().GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "").Return(s.caveats(now))
	s.bakery.EXPECT().NewMacaroon(
		gomock.Any(),
		bakery.LatestVersion,
		s.caveats(now),
		crossModelConsumeOp("mysql-uuid"),
	).Return(expected, nil)

	authContext := s.newAuthContext(c)
	mac, err := authContext.CreateConsumeOfferMacaroon(c.Context(), s.modelUUID, "mysql-uuid", "mary", bakery.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mac, tc.Equals, expected)
}

func (s *authSuite) TestCreateRemoteRelationMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	expected := &bakery.Macaroon{}

	s.bakery.EXPECT().GetRemoteRelationCaveats("mysql-uuid", s.modelUUID.String(), "mary", "relation-mediawiki.db#mysql.server").Return(s.caveatWithRelation(now))
	s.bakery.EXPECT().NewMacaroon(
		gomock.Any(),
		bakery.LatestVersion,
		s.caveatWithRelation(now),
		crossModelRelateOp("relation-mediawiki.db#mysql.server"),
	).Return(expected, nil)

	authContext := s.newAuthContext(c)
	mac, err := authContext.CreateRemoteRelationMacaroon(c.Context(), s.modelUUID, "mysql-uuid", "mary", names.NewRelationTag("mediawiki:db mysql:server"), bakery.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mac, tc.Equals, expected)
}

func (s *authSuite) TestAuthenticator(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	authenticator := authContext.Authenticator()
	c.Assert(authenticator, tc.Not(tc.IsNil))
}

func (s *authSuite) newAccessCaveat(modelUUID string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], modelUUID)
}

func (s *authSuite) newAccessCaveatWithUser(user string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: %s
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], s.modelUUID.String(), user)
}

func (s *authSuite) newAccessCaveatWithPermission(permission string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: %s
`[1:], s.modelUUID.String(), permission)
}

func (s *authSuite) newOfferAccessDetails() crossmodelbakery.OfferAccessDetails {
	return crossmodelbakery.OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	}
}

func (s *authSuite) newAuthContext(c *tc.C) *AuthContext {
	return NewAuthContext(
		s.accessService,
		s.bakery,
		s.keyPair,
		s.controllerUUID,
		s.modelUUID,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *authSuite) caveats(now time.Time) []checkers.Caveat {
	return []checkers.Caveat{
		checkers.DeclaredCaveat("source-model-uuid", s.modelUUID.String()),
		checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
		checkers.DeclaredCaveat("username", "mary"),
		checkers.TimeBeforeCaveat(now.Add(time.Minute * 3)),
	}
}

func (s *authSuite) caveatWithRelation(now time.Time) []checkers.Caveat {
	return append(s.caveats(now), checkers.DeclaredCaveat("relation-key", "mediawiki:db mysql:server"))
}

func (s *authSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.accessService = NewMockAccessService(ctrl)
	s.bakery = NewMockOfferBakery(ctrl)

	s.keyPair = bakery.MustGenerateKey()

	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = modeltesting.GenModelUUID(c)

	c.Cleanup(func() {
		s.clock = nil
		s.accessService = nil
		s.bakery = nil
		s.keyPair = nil

		s.controllerUUID = ""
		s.modelUUID = ""
	})

	return ctrl
}
