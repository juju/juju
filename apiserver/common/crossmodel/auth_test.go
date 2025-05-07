// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/crossmodel/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&authSuite{})

type authSuite struct {
	coretesting.BaseSuite

	bakery        authentication.ExpirableStorageBakery
	offerBakery   *crossmodel.OfferBakery
	bakeryKey     *bakery.KeyPair
	accessService *mocks.MockAccessService
}

type testLocator struct {
	PublicKey bakery.PublicKey
}

func (b testLocator) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	if loc != "http://thirdparty" {
		return bakery.ThirdPartyInfo{}, errors.NotFoundf("location %v", loc)
	}
	return bakery.ThirdPartyInfo{
		PublicKey: b.PublicKey,
		Version:   bakery.LatestVersion,
	}, nil
}

func (s *authSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	key, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)
	locator := testLocator{key.Public}
	bakery := bakery.New(bakery.BakeryParams{
		Locator:       locator,
		Key:           bakery.MustGenerateKey(),
		OpsAuthorizer: crossmodel.CrossModelAuthorizer{},
	})
	s.bakery = &mockBakery{bakery}
	s.offerBakery = crossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock)
}

func (s *authSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = mocks.NewMockAccessService(ctrl)
	return ctrl
}

func (s *authSuite) TestCheckValidCaveat(c *tc.C) {
	uuid := uuid.MustNewUUID()
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	authContext, err := crossmodel.NewAuthContext(nil, nil, names.NewModelTag(uuid.String()), s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opc.SourceModelUUID, tc.Equals, uuid.String())
	c.Assert(opc.User, tc.Equals, "mary")
	c.Assert(opc.OfferUUID, tc.Equals, "mysql-uuid")
	c.Assert(opc.Relation, tc.Equals, "mediawiki:db mysql:server")
	c.Assert(opc.Permission, tc.Equals, "consume")
}

func (s *authSuite) TestCheckInvalidCaveatId(c *tc.C) {
	uuid := uuid.MustNewUUID()
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	authContext, err := crossmodel.NewAuthContext(nil, nil, names.NewModelTag(uuid.String()), s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckOfferAccessCaveat("different-caveat " + permCheckDetails)
	c.Assert(err, tc.ErrorMatches, ".*caveat not recognized.*")
}

func (s *authSuite) TestCheckInvalidCaveatContents(c *tc.C) {
	permCheckDetails := `
source-model-uuid: invalid
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:]
	authContext, err := crossmodel.NewAuthContext(nil, nil, names.NewModelTag("invalid"), s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, tc.ErrorMatches, `source-model-uuid "invalid" not valid`)
}

func (s *authSuite) TestCheckLocalAccessRequest(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	uuid := uuid.MustNewUUID()
	st := &mockState{}
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        uuid.String(),
	}).Return(permission.NoAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        coretesting.ControllerTag.Id(),
	}).Return(permission.NoAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Offer,
		Key:        "mysql-uuid",
	}).Return(permission.ConsumeAccess, nil)
	authContext, err := crossmodel.NewAuthContext(st, s.accessService, names.NewModelTag(uuid.String()), s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	cav, err := authContext.CheckLocalAccessRequest(context.Background(), opc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cav, tc.HasLen, 5)
	c.Assert(cav[0].Condition, tc.Equals, "declared source-model-uuid "+uuid.String())
	c.Assert(cav[1].Condition, tc.Equals, "declared offer-uuid mysql-uuid")
	c.Assert(cav[2].Condition, tc.Equals, "declared username mary")
	c.Assert(strings.HasPrefix(cav[3].Condition, "time-before"), jc.IsTrue)
	c.Assert(cav[4].Condition, tc.Equals, "declared relation-key mediawiki:db mysql:server")
}

func (s *authSuite) TestCheckLocalAccessRequestControllerAdmin(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	uuid := uuid.MustNewUUID()
	st := &mockState{}
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        coretesting.ControllerTag.Id(),
	}).Return(permission.SuperuserAccess, nil)
	authContext, err := crossmodel.NewAuthContext(st, s.accessService, names.NewModelTag(uuid.String()), s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckLocalAccessRequest(context.Background(), opc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckLocalAccessRequestModelAdmin(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	uuid := uuid.MustNewUUID()
	st := &mockState{}
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        coretesting.ControllerTag.Id(),
	}).Return(permission.NoAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        uuid.String(),
	}).Return(permission.AdminAccess, nil)
	authContext, err := crossmodel.NewAuthContext(st, s.accessService, names.NewModelTag(uuid.String()), s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckLocalAccessRequest(context.Background(), opc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckLocalAccessRequestNoPermission(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	uuid := uuid.MustNewUUID()
	st := &mockState{}
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Controller,
		Key:        coretesting.ControllerTag.Id(),
	}).Return(permission.NoAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Model,
		Key:        uuid.String(),
	}).Return(permission.NoAccess, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), usertesting.GenNewName(c, "mary"), permission.ID{
		ObjectType: permission.Offer,
		Key:        "mysql-uuid",
	}).Return(permission.NoAccess, nil)
	authContext, err := crossmodel.NewAuthContext(st, s.accessService, names.NewModelTag(uuid.String()), s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckLocalAccessRequest(context.Background(), opc)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCreateConsumeOfferMacaroon(c *tc.C) {
	offer := &params.ApplicationOfferDetailsV5{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferUUID:      "mysql-uuid",
	}
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := authContext.CreateConsumeOfferMacaroon(context.Background(), offer, "mary", bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.M().Caveats()
	c.Assert(cav, tc.HasLen, 4)
	c.Assert(bytes.HasPrefix(cav[0].Id, []byte("time-before")), jc.IsTrue)
	c.Assert(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid "+coretesting.ModelTag.Id()))
	c.Assert(cav[2].Id, jc.DeepEquals, []byte("declared username mary"))
	c.Assert(cav[3].Id, jc.DeepEquals, []byte("declared offer-uuid mysql-uuid"))
}

func (s *authSuite) TestCreateRemoteRelationMacaroon(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := authContext.CreateRemoteRelationMacaroon(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()), "mysql-uuid", "mary", names.NewRelationTag("mediawiki:db mysql:server"), bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.M().Caveats()
	c.Assert(cav, tc.HasLen, 5)
	c.Assert(bytes.HasPrefix(cav[0].Id, []byte("time-before")), jc.IsTrue)
	c.Assert(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid "+coretesting.ModelTag.Id()))
	c.Assert(cav[2].Id, jc.DeepEquals, []byte("declared offer-uuid mysql-uuid"))
	c.Assert(cav[3].Id, jc.DeepEquals, []byte("declared username mary"))
	c.Assert(cav[4].Id, jc.DeepEquals, []byte("declared relation-key mediawiki:db mysql:server"))
}

func (s *authSuite) TestCheckOfferMacaroons(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	attr, err := authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attr, tc.HasLen, 3)
	c.Assert(attr, jc.DeepEquals, map[string]string{
		"username":          "mary",
		"offer-uuid":        "mysql-uuid",
		"source-model-uuid": coretesting.ModelTag.Id(),
	})
}

func (s *authSuite) TestCheckOfferMacaroonsWrongOffer(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"prod.another",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(
		err,
		tc.ErrorMatches,
		"permission denied")
}

func (s *authSuite) TestCheckOfferMacaroonsNoUser(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckOfferMacaroonsDischargeRequired(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext.SetClock(clock)
	authContext, err = authContext.WithDischargeURL(context.Background(), "http://thirdparty")
	c.Assert(err, jc.ErrorIsNil)
	offer := &params.ApplicationOfferDetailsV5{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferUUID:      "mysql-uuid",
	}
	mac, err := authContext.CreateConsumeOfferMacaroon(context.Background(), offer, "mary", bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	_, err = authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	dischargeErr, ok := err.(*apiservererrors.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, tc.HasLen, 2)
	c.Assert(cav[0].Location, tc.Equals, "http://thirdparty")
}

func (s *authSuite) TestCheckRelationMacaroons(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"relate", relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"mysql-uuid",
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckRelationMacaroonsWrongRelation(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"relate", relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"mysql-uuid",
		names.NewRelationTag("app:db offer:db"),
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(
		err,
		tc.ErrorMatches,
		"permission denied")
}

func (s *authSuite) TestCheckRelationMacaroonsNoUser(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"relate", relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"mysql-uuid",
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckRelationMacaroonsDischargeRequired(c *tc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, nil, coretesting.ModelTag, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext.SetClock(clock)
	authContext, err = authContext.WithDischargeURL(context.Background(), "http://thirdparty")
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := authContext.CreateRemoteRelationMacaroon(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()), "mysql-uuid", "mary", relationTag, bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		model.UUID(coretesting.ModelTag.Id()),
		"mysql-uuid",
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	dischargeErr, ok := err.(*apiservererrors.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, tc.HasLen, 2)
	c.Assert(cav[0].Location, tc.Equals, "http://thirdparty")
}
