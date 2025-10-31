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
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&authSuite{})

type authSuite struct {
	coretesting.BaseSuite

	locator     testLocator
	bakery      authentication.ExpirableStorageBakery
	offerBakery *crossmodel.OfferBakery
	bakeryKey   *bakery.KeyPair
}

type testLocator struct {
	PublicKey  bakery.PublicKey
	PrivateKey bakery.PrivateKey
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

func (s *authSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	key, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)
	s.locator = testLocator{PublicKey: key.Public, PrivateKey: key.Private}
	bakery := bakery.New(bakery.BakeryParams{
		Locator: s.locator,
		Key:     bakery.MustGenerateKey(),
	})
	s.bakery = &mockBakery{bakery}
	s.offerBakery = crossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock)
}

func (s *authSuite) TestCheckValidCaveat(c *gc.C) {
	uuid := utils.MustNewUUID()
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opc.SourceModelUUID, gc.Equals, uuid.String())
	c.Assert(opc.User, gc.Equals, "mary")
	c.Assert(opc.OfferUUID, gc.Equals, "mysql-uuid")
	c.Assert(opc.Relation, gc.Equals, "mediawiki:db mysql:server")
	c.Assert(opc.Permission, gc.Equals, "consume")
}

func (s *authSuite) TestCheckInvalidCaveatId(c *gc.C) {
	uuid := utils.MustNewUUID()
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckOfferAccessCaveat("different-caveat " + permCheckDetails)
	c.Assert(err, gc.ErrorMatches, ".*caveat not recognized.*")
}

func (s *authSuite) TestCheckInvalidCaveatContents(c *gc.C) {
	permCheckDetails := `
source-model-uuid: invalid
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:]
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, gc.ErrorMatches, `source-model-uuid "invalid" not valid`)
}

func (s *authSuite) TestCheckLocalAccessRequest(c *gc.C) {
	uuid := utils.MustNewUUID()
	st := &mockState{
		tag: names.NewModelTag(uuid.String()),
		permissions: map[string]permission.Access{
			"mysql-uuid:mary": permission.ConsumeAccess,
		},
	}
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.offerBakery)
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
	cav, err := authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cav, gc.HasLen, 5)
	c.Assert(cav[0].Condition, gc.Equals, "declared source-model-uuid "+uuid.String())
	c.Assert(cav[1].Condition, gc.Equals, "declared offer-uuid mysql-uuid")
	c.Assert(cav[2].Condition, gc.Equals, "declared username mary")
	c.Assert(strings.HasPrefix(cav[3].Condition, "time-before"), jc.IsTrue)
	c.Assert(cav[4].Condition, gc.Equals, "declared relation-key mediawiki:db mysql:server")
}

func (s *authSuite) TestCheckLocalAccessRequestControllerAdmin(c *gc.C) {
	uuid := utils.MustNewUUID()
	st := &mockState{
		tag: names.NewModelTag(uuid.String()),
		permissions: map[string]permission.Access{
			coretesting.ControllerTag.Id() + ":mary": permission.SuperuserAccess,
		},
	}
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.offerBakery)
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
	_, err = authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckLocalAccessRequestModelAdmin(c *gc.C) {
	uuid := utils.MustNewUUID()
	st := &mockState{
		tag: names.NewModelTag(uuid.String()),
		permissions: map[string]permission.Access{
			uuid.String() + ":mary": permission.AdminAccess,
		},
	}
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.offerBakery)
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
	_, err = authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckLocalAccessRequestNoPermission(c *gc.C) {
	uuid := utils.MustNewUUID()
	st := &mockState{
		tag:         names.NewModelTag(uuid.String()),
		permissions: make(map[string]permission.Access),
	}
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.offerBakery)
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
	_, err = authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCreateConsumeOfferMacaroon(c *gc.C) {
	offer := &params.ApplicationOfferDetailsV5{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferUUID:      "mysql-uuid",
	}
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := authContext.CreateConsumeOfferMacaroon(context.Background(), offer, "mary", bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.M().Caveats()
	c.Assert(cav, gc.HasLen, 4)
	c.Assert(bytes.HasPrefix(cav[0].Id, []byte("time-before")), jc.IsTrue)
	c.Assert(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid "+coretesting.ModelTag.Id()))
	c.Assert(cav[2].Id, jc.DeepEquals, []byte("declared username mary"))
	c.Assert(cav[3].Id, jc.DeepEquals, []byte("declared offer-uuid mysql-uuid"))
}

func (s *authSuite) TestCreateRemoteRelationMacaroon(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := authContext.CreateRemoteRelationMacaroon(
		context.Background(),
		coretesting.ModelTag.Id(), "mysql-uuid", "mary", names.NewRelationTag("mediawiki:db mysql:server"), bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.M().Caveats()
	c.Assert(cav, gc.HasLen, 5)
	c.Assert(bytes.HasPrefix(cav[0].Id, []byte("time-before")), jc.IsTrue)
	c.Assert(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid "+coretesting.ModelTag.Id()))
	c.Assert(cav[2].Id, jc.DeepEquals, []byte("declared offer-uuid mysql-uuid"))
	c.Assert(cav[3].Id, jc.DeepEquals, []byte("declared username mary"))
	c.Assert(cav[4].Id, jc.DeepEquals, []byte("declared relation-key mediawiki:db mysql:server"))
}

func (s *authSuite) TestCheckOfferMacaroons(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{Action: "consume", Entity: "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	attr, err := authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attr, gc.HasLen, 3)
	c.Assert(attr, jc.DeepEquals, map[string]string{
		"username":          "mary",
		"offer-uuid":        "mysql-uuid",
		"source-model-uuid": coretesting.ModelTag.Id(),
	})
}

func (s *authSuite) TestCheckOfferMacaroonsWithFakeMacaroon(c *gc.C) {
	// set up the auth context as usual
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext.SetClock(clock)
	authContext, err = authContext.WithDischargeURL("http://thirdparty")
	c.Assert(err, jc.ErrorIsNil)

	// create a new bakery used to mint a fake macaroon
	key, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)
	locator := testLocator{PublicKey: key.Public, PrivateKey: key.Private}
	attackerBakery := &mockBakery{bakery.New(bakery.BakeryParams{
		Locator: locator,
		Key:     bakery.MustGenerateKey(),
	})}

	// mint a fake macaroon declaring all the caveats we expect
	mac, err := attackerBakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "offerowner"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{Action: "consume", Entity: "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)

	// verify the fake macaroon
	_, err = authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)

	/* Before this change we would get a discharge required error
	   addressed to the controller that required the controller to
	   confirm the user stated in the fake macaroon had access to the offer.
	   Assuming a malicious actor knew who had access to the offer they
	   would be able to gain access themselves by presenting a fake macaroon
	   only to get it replaced by a valid one once the discharge was done.

	   I am leaving this code commented out for reference and a warning
	   to future developers dealing with macaroons and cross-model auth.

		dischargeErr, ok := err.(*apiservererrors.DischargeRequiredError)
		c.Assert(ok, jc.IsTrue)
		cav := dischargeErr.LegacyMacaroon.Caveats()
		c.Assert(cav, gc.HasLen, 2)
		c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")

		// Let's see what the we're asking the controller to discharge
		checker := &mockChecker{}
		_, err = bakery.DischargeAll(context.Background(), dischargeErr.Macaroon, func(ctx context.Context, cav macaroon.Caveat, payload []byte) (*bakery.Macaroon, error) {
			return discharge(ctx, &bakery.KeyPair{Public: s.locator.PublicKey, Private: s.locator.PrivateKey}, checker, cav, payload)
		})
		c.Assert(err, jc.ErrorIsNil)
		expectedString := "" +
			"has-offer-permission source-model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d\n" +
			"username: offerowner\n" +
			"offer-uuid: mysql-uuid\n" +
			"relation-key: \"\"\n" +
			"permission: consume\n"
		c.Assert(checker.condition, gc.Equals, expectedString)

		// A-ha! we faked the macaroon, but now we're we're getting a discharge error
		// asking the controller to confirm that the offer owner has access to the offer
		// although we may not be the offer owner!!!

		func discharge(ctx context.Context, key *bakery.KeyPair, checker bakery.ThirdPartyCaveatChecker, cav macaroon.Caveat, payload []byte) (*bakery.Macaroon, error) {
			return bakery.Discharge(ctx, bakery.DischargeParams{
				Key:     key,
				Id:      cav.Id,
				Caveat:  payload,
				Checker: checker,
			})
		}

		type mockChecker struct {
			condition string
		}

		func (m *mockChecker) CheckThirdPartyCaveat(ctx context.Context, info *bakery.ThirdPartyCaveatInfo) ([]checkers.Caveat, error) {
			m.condition = string(info.Condition)
			return []checkers.Caveat{{
				Condition: "declared attcher-is right",
			}}, nil
		}
	*/
}

func (s *authSuite) TestCheckOfferMacaroonsWrongOffer(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{Action: "consume", Entity: "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"prod.another",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(
		err,
		gc.ErrorMatches,
		"permission denied")
}

func (s *authSuite) TestCheckOfferMacaroonsNoUser(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{Action: "consume", Entity: "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckOfferMacaroonsDischargeRequired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext.SetClock(clock)
	authContext, err = authContext.WithDischargeURL("http://thirdparty")
	c.Assert(err, jc.ErrorIsNil)
	offer := &params.ApplicationOfferDetailsV5{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferUUID:      "mysql-uuid",
	}
	mac, err := authContext.CreateConsumeOfferMacaroon(context.Background(), offer, "mary", bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	_, err = authContext.Authenticator().CheckOfferMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	dischargeErr, ok := err.(*apiservererrors.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}

func (s *authSuite) TestCheckRelationMacaroons(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{Action: "relate", Entity: relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckRelationMacaroonsWrongRelation(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{Action: "relate", Entity: relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		names.NewRelationTag("app:db offer:db"),
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(
		err,
		gc.ErrorMatches,
		"permission denied")
}

func (s *authSuite) TestCheckRelationMacaroonsNoUser(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{Action: "relate", Entity: relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckRelationMacaroonsDischargeRequired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.offerBakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext.SetClock(clock)
	authContext, err = authContext.WithDischargeURL("http://thirdparty")
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := authContext.CreateRemoteRelationMacaroon(
		context.Background(),
		coretesting.ModelTag.Id(), "mysql-uuid", "mary", relationTag, bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = authContext.Authenticator().CheckRelationMacaroons(
		context.Background(),
		coretesting.ModelTag.Id(),
		"mysql-uuid",
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	dischargeErr, ok := err.(*apiservererrors.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}
