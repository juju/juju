// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&authSuite{})

type authSuite struct {
	coretesting.BaseSuite

	authContext   *crossmodel.AuthContext
	bakery        authentication.ExpirableStorageBakery
	bakeryKey     *bakery.KeyPair
	mockStatePool *mockStatePool
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

func (s *authSuite) SetUpTest(c *gc.C) {
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
	s.mockStatePool = &mockStatePool{st: make(map[string]crossmodel.Backend)}
	s.authContext, err = crossmodel.NewAuthContext(s.mockStatePool, s.bakeryKey, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
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
	opc, err := s.authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
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
	_, err := s.authContext.CheckOfferAccessCaveat("different-caveat " + permCheckDetails)
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
	_, err := s.authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
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
	s.mockStatePool.st[uuid.String()] = st
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := s.authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	cav, err := s.authContext.CheckLocalAccessRequest(opc)
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
	s.mockStatePool.st[uuid.String()] = st
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := s.authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.authContext.CheckLocalAccessRequest(opc)
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
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := s.authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckLocalAccessRequestNoPermission(c *gc.C) {
	uuid := utils.MustNewUUID()
	st := &mockState{
		tag:         names.NewModelTag(uuid.String()),
		permissions: make(map[string]permission.Access),
	}
	s.mockStatePool.st[uuid.String()] = st
	permCheckDetails := fmt.Sprintf(`
source-model-uuid: %v
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], uuid)
	opc, err := s.authContext.CheckOfferAccessCaveat("has-offer-permission " + permCheckDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.authContext.CheckLocalAccessRequest(opc)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCreateConsumeOfferMacaroon(c *gc.C) {
	offer := &params.ApplicationOfferDetails{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferUUID:      "mysql-uuid",
	}
	mac, err := s.authContext.CreateConsumeOfferMacaroon(context.TODO(), offer, "mary", bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.M().Caveats()
	c.Assert(cav, gc.HasLen, 4)
	c.Assert(bytes.HasPrefix(cav[0].Id, []byte("time-before")), jc.IsTrue)
	c.Assert(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid "+coretesting.ModelTag.Id()))
	c.Assert(cav[2].Id, jc.DeepEquals, []byte("declared offer-uuid mysql-uuid"))
	c.Assert(cav[3].Id, jc.DeepEquals, []byte("declared username mary"))
}

func (s *authSuite) TestCreateRemoteRelationMacaroon(c *gc.C) {
	mac, err := s.authContext.CreateRemoteRelationMacaroon(
		context.TODO(),
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
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	attr, err := s.authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckOfferMacaroons(
		context.TODO(),
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

func (s *authSuite) TestCheckOfferMacaroonsWrongOffer(c *gc.C) {
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = s.authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckOfferMacaroons(
		context.TODO(),
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
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = s.authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckOfferMacaroons(
		context.TODO(),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckOfferMacaroonsDischargeRequired(c *gc.C) {
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext := s.authContext.WithClock(clock)
	authContext = authContext.WithDischargeURL("http://thirdparty")
	offer := &params.ApplicationOfferDetails{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferUUID:      "mysql-uuid",
	}
	mac, err := authContext.CreateConsumeOfferMacaroon(context.TODO(), offer, "mary", bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckOfferMacaroons(
		context.TODO(),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	dischargeErr, ok := err.(*common.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}

func (s *authSuite) TestCheckRelationMacaroons(c *gc.C) {
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"relate", relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = s.authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckRelationMacaroons(
		context.TODO(),
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckRelationMacaroonsWrongRelation(c *gc.C) {
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"relate", relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = s.authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckRelationMacaroons(
		context.TODO(),
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
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"relate", relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = s.authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckRelationMacaroons(
		context.TODO(),
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckRelationMacaroonsDischargeRequired(c *gc.C) {
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext := s.authContext.WithClock(clock)
	authContext = authContext.WithDischargeURL("http://thirdparty")
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := authContext.CreateRemoteRelationMacaroon(
		context.TODO(),
		coretesting.ModelTag.Id(), "mysql-uuid", "mary", relationTag, bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckRelationMacaroons(
		context.TODO(),
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	dischargeErr, ok := err.(*common.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}
