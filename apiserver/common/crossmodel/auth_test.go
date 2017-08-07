// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&authSuite{})

type authSuite struct {
	coretesting.BaseSuite

	bakery        authentication.ExpirableStorageBakeryService
	mockStatePool *mockStatePool
}

func (s *authSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	key, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)
	bakery, err := bakery.NewService(bakery.NewServiceParams{
		Locator: bakery.PublicKeyLocatorMap{
			"http://thirdparty": &key.Public,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.bakery = &mockBakeryService{bakery}
	s.mockStatePool = &mockStatePool{st: make(map[string]crossmodel.Backend)}
}

func (s *authSuite) TestCheckValidCaveat(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	uuid := utils.MustNewUUID()
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-url: prod.mysql
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opc.SourceModelUUID, gc.Equals, uuid.String())
	c.Assert(opc.User, gc.Equals, "mary")
	c.Assert(opc.Offer, gc.Equals, "prod.mysql")
	c.Assert(opc.Relation, gc.Equals, "mediawiki:db mysql:server")
	c.Assert(opc.Permission, gc.Equals, "consume")
}

func (s *authSuite) TestCheckInvalidCaveatId(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	uuid := utils.MustNewUUID()
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-url: prod.mysql
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	_, err = authContext.CheckOfferAccessCaveat("different-caveat " + permCheckDetails)
	c.Assert(err, gc.ErrorMatches, ".*caveat not recognized.*")
}

func (s *authSuite) TestCheckInvalidCaveatContents(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := `
source-model-uuid: invalid
username: mary
offer-url: prod.mysql
relation-key: mediawiki:db mysql:server
permission: consume
`[1:]
	_, err = authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, gc.ErrorMatches, `source-model-uuid "invalid" not valid`)
}

func (s *authSuite) TestCheckLocalAccessRequest(c *gc.C) {
	uuid := utils.MustNewUUID()
	st := &mockState{
		tag: names.NewModelTag(uuid.String()),
		permissions: map[string]permission.Access{
			"prod.hosted-mysql:mary": permission.ConsumeAccess,
		},
	}
	s.mockStatePool.st[uuid.String()] = st
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-url: prod.hosted-mysql
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	cav, err := authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cav, gc.HasLen, 5)
	c.Assert(cav[0].Condition, gc.Equals, "declared source-model-uuid "+uuid.String())
	c.Assert(cav[1].Condition, gc.Equals, "declared offer-url prod.hosted-mysql")
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
	s.mockStatePool.st[uuid.String()] = st
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-url: prod.hosted-mysql
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
	s.mockStatePool.st[uuid.String()] = st
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-url: prod.hosted-mysql
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
	s.mockStatePool.st[uuid.String()] = st
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-url: prod.hosted-mysql
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCreateConsumeOfferMacaroon(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	offer := &params.ApplicationOffer{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferURL:       "prod.hosted-mysql",
	}
	mac, err := authContext.CreateConsumeOfferMacaroon(offer, "mary")
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.Caveats()
	c.Assert(cav, gc.HasLen, 4)
	c.Assert(strings.HasPrefix(cav[0].Id, "time-before"), jc.IsTrue)
	c.Assert(cav[1].Id, gc.Equals, "declared source-model-uuid "+coretesting.ModelTag.Id())
	c.Assert(cav[2].Id, gc.Equals, "declared offer-url prod.hosted-mysql")
	c.Assert(cav[3].Id, gc.Equals, "declared username mary")
}

func (s *authSuite) TestCreateRemoteRelationMacaroon(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := authContext.CreateRemoteRelationMacaroon(
		coretesting.ModelTag.Id(), "prod.hosted-mysql", "mary", names.NewRelationTag("mediawiki:db mysql:server"))
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.Caveats()
	c.Assert(cav, gc.HasLen, 5)
	c.Assert(strings.HasPrefix(cav[0].Id, "time-before"), jc.IsTrue)
	c.Assert(cav[1].Id, gc.Equals, "declared source-model-uuid "+coretesting.ModelTag.Id())
	c.Assert(cav[2].Id, gc.Equals, "declared offer-url prod.hosted-mysql")
	c.Assert(cav[3].Id, gc.Equals, "declared username mary")
	c.Assert(cav[4].Id, gc.Equals, "declared relation-key mediawiki:db mysql:server")
}

func (s *authSuite) TestCheckOfferMacaroons(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat("username", "mary"),
		checkers.DeclaredCaveat("offer-url", "prod.hosted-mysql"),
		checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
	})
	c.Assert(err, jc.ErrorIsNil)
	attr, err := authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckOfferMacaroons(
		"prod.hosted-mysql",
		macaroon.Slice{mac},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attr, gc.HasLen, 3)
	c.Assert(attr, jc.DeepEquals, map[string]string{
		"username":          "mary",
		"offer-url":         "prod.hosted-mysql",
		"source-model-uuid": coretesting.ModelTag.Id(),
	})
}

func (s *authSuite) TestCheckOfferMacaroonsWrongOffer(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat("username", "mary"),
		checkers.DeclaredCaveat("offer-url", "prod.hosted-mysql"),
		checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckOfferMacaroons(
		"prod.another",
		macaroon.Slice{mac},
	)
	c.Assert(
		err,
		gc.ErrorMatches,
		`.*caveat "declared offer-url prod.hosted-mysql" not satisfied: got offer-url="prod.another", expected "prod.hosted-mysql"`)
}

func (s *authSuite) TestCheckOfferMacaroonsNoUser(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat("offer-url", "prod.hosted-mysql"),
		checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckOfferMacaroons(
		"prod.hosted-mysql",
		macaroon.Slice{mac},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckOfferMacaroonsExpired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testing.NewClock(time.Now().Add(-10 * time.Minute))
	authContext = authContext.WithClock(clock)
	offer := &params.ApplicationOffer{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferURL:       "prod.hosted-mysql",
	}
	mac, err := authContext.CreateConsumeOfferMacaroon(offer, "mary")
	c.Assert(err, jc.ErrorIsNil)

	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckOfferMacaroons(
		"prod.hosted-mysql",
		macaroon.Slice{mac},
	)
	c.Assert(err, gc.ErrorMatches, ".*macaroon has expired")
}

func (s *authSuite) TestCheckOfferMacaroonsDischargeRequired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testing.NewClock(time.Now().Add(-10 * time.Minute))
	authContext = authContext.WithClock(clock)
	authContext = authContext.WithDischargeURL("http://thirdparty")
	offer := &params.ApplicationOffer{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferURL:       "prod.hosted-mysql",
	}
	mac, err := authContext.CreateConsumeOfferMacaroon(offer, "mary")
	c.Assert(err, jc.ErrorIsNil)

	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckOfferMacaroons(
		"prod.hosted-mysql",
		macaroon.Slice{mac},
	)
	dischargeErr, ok := err.(*common.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.Macaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}

func (s *authSuite) TestCheckRelationMacaroons(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat("username", "mary"),
		checkers.DeclaredCaveat("relation-key", relationTag.Id()),
		checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckRelationMacaroons(
		relationTag,
		macaroon.Slice{mac},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckRelationMacaroonsWrongRelation(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat("username", "mary"),
		checkers.DeclaredCaveat("relation-key", names.NewRelationTag("mediawiki:db mysql:server").Id()),
		checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckRelationMacaroons(
		names.NewRelationTag("app:db offer:db"),
		macaroon.Slice{mac},
	)
	c.Assert(
		err,
		gc.ErrorMatches,
		`.*caveat "declared relation-key mediawiki:db mysql:server" not satisfied: got relation-key="app:db offer:db", expected "mediawiki:db mysql:server"`)
}

func (s *authSuite) TestCheckRelationMacaroonsNoUser(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat("relation-key", relationTag.Id()),
		checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckRelationMacaroons(
		relationTag,
		macaroon.Slice{mac},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckRelationMacaroonsExpired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testing.NewClock(time.Now().Add(-10 * time.Minute))
	authContext = authContext.WithClock(clock)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := authContext.CreateRemoteRelationMacaroon(
		coretesting.ModelTag.Id(), "prod.offer", "mary", relationTag)
	c.Assert(err, jc.ErrorIsNil)

	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckOfferMacaroons(
		"prod.hosted-mysql",
		macaroon.Slice{mac},
	)
	c.Assert(err, gc.ErrorMatches, ".*macaroon has expired")
}

func (s *authSuite) TestCheckRelationMacaroonsDischargeRequired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(s.mockStatePool, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	clock := testing.NewClock(time.Now().Add(-10 * time.Minute))
	authContext = authContext.WithClock(clock)
	authContext = authContext.WithDischargeURL("http://thirdparty")
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := authContext.CreateRemoteRelationMacaroon(
		coretesting.ModelTag.Id(), "prod.offer", "mary", relationTag)
	c.Assert(err, jc.ErrorIsNil)

	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "prod.hosted-mysql").CheckOfferMacaroons(
		"prod.hosted-mysql",
		macaroon.Slice{mac},
	)
	dischargeErr, ok := err.(*common.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.Macaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}
