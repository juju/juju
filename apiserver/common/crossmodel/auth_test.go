// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&authSuite{})

type authSuite struct {
	coretesting.BaseSuite

	bakery    authentication.ExpirableStorageBakery
	bakeryKey *bakery.KeyPair
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
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
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
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
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
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
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
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.bakery, nil, nil)
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
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.bakery, nil, nil)
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
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.bakery, nil, nil)
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
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.bakery, nil, nil)
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
	offer := &params.ApplicationOfferDetails{
		SourceModelTag: coretesting.ModelTag.String(),
		OfferUUID:      "mysql-uuid",
	}
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := authContext.CreateConsumeOfferMacaroon(context.TODO(), offer, "mary", bakery.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	cav := mac.M().Caveats()
	c.Assert(cav, gc.HasLen, 4)
	c.Assert(bytes.HasPrefix(cav[0].Id, []byte("time-before")), jc.IsTrue)
	c.Assert(cav[1].Id, jc.DeepEquals, []byte("declared source-model-uuid "+coretesting.ModelTag.Id()))
	c.Assert(cav[2].Id, jc.DeepEquals, []byte("declared offer-uuid mysql-uuid"))
	c.Assert(cav[3].Id, jc.DeepEquals, []byte("declared username mary"))
}

func (s *authSuite) TestCreateRemoteRelationMacaroon(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := authContext.CreateRemoteRelationMacaroon(
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
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	attr, err := authContext.Authenticator(
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
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator(
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
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("offer-uuid", "mysql-uuid"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})

	c.Assert(err, jc.ErrorIsNil)
	_, err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckOfferMacaroons(
		context.TODO(),
		"mysql-uuid",
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckOfferMacaroonsDischargeRequired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext = authContext.WithClock(clock)
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
	dischargeErr, ok := err.(*apiservererrors.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}

func (s *authSuite) TestCheckRelationMacaroons(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
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
	err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckRelationMacaroons(
		context.TODO(),
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authSuite) TestCheckRelationMacaroonsWrongRelation(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
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
	err = authContext.Authenticator(
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
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	relationTag := names.NewRelationTag("mediawiki:db mysql:server")
	mac, err := s.bakery.NewMacaroon(
		context.TODO(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("relation-key", relationTag.Id()),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
		}, bakery.Op{"relate", relationTag.Id()})

	c.Assert(err, jc.ErrorIsNil)
	err = authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckRelationMacaroons(
		context.TODO(),
		relationTag,
		macaroon.Slice{mac.M()},
		bakery.LatestVersion,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authSuite) TestCheckRelationMacaroonsDischargeRequired(c *gc.C) {
	authContext, err := crossmodel.NewAuthContext(nil, s.bakeryKey, s.bakery, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	clock := testclock.NewClock(time.Now().Add(-10 * time.Minute))
	authContext = authContext.WithClock(clock)
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
	dischargeErr, ok := err.(*apiservererrors.DischargeRequiredError)
	c.Assert(ok, jc.IsTrue)
	cav := dischargeErr.LegacyMacaroon.Caveats()
	c.Assert(cav, gc.HasLen, 2)
	c.Assert(cav[0].Location, gc.Equals, "http://thirdparty")
}

type tokenEntity struct {
	user names.UserTag
}

func (t tokenEntity) Tag() names.Tag {
	return t.user
}

type tokenParser struct {
	jwkSet jwk.Set
}

func (p *tokenParser) Parse(ctx context.Context, tok string) (jwt.Token, state.Entity, error) {
	tokBytes, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, nil, errors.Annotate(err, "invalid jwt authToken in request")
	}

	jwtTok, err := jwt.Parse(
		tokBytes,
		jwt.WithKeySet(p.jwkSet),
	)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	userTag, err := names.ParseUserTag(jwtTok.Subject())
	if err != nil {
		return nil, nil, err
	}
	return jwtTok, tokenEntity{userTag}, err
}

var permissionFunc = func(token jwt.Token, subject names.Tag) (permission.Access, error) {
	accessClaims, ok := token.PrivateClaims()["access"].(map[string]interface{})
	if !ok || len(accessClaims) == 0 {
		return permission.NoAccess, nil
	}
	access, ok := accessClaims[subject.String()]
	if !ok {
		return permission.NoAccess, nil
	}
	return permission.Access(fmt.Sprintf("%v", access)), nil
}

func (s *authSuite) TestCheckCheckOfferToken(c *gc.C) {
	jwkSet, sign, err := testing.NewJWKSet()
	c.Assert(err, jc.ErrorIsNil)
	parser := &tokenParser{jwkSet}
	tokBytes, err := testing.EncodedJWT(testing.JWTParams{
		Controller: coretesting.ControllerTag.String(),
		User:       "user-fred",
		Access: map[string]string{
			"applicationoffer-mysql-uuid": "consume",
		},
	}, jwkSet, sign)
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID()
	st := &mockState{
		tag: names.NewModelTag(uuid.String()),
	}
	authContext, err := crossmodel.NewAuthContext(st, s.bakeryKey, s.bakery, parser, permissionFunc)
	c.Assert(err, jc.ErrorIsNil)
	username, err := authContext.Authenticator(
		coretesting.ModelTag.Id(), "mysql-uuid").CheckOfferToken(
		context.TODO(),
		base64.StdEncoding.EncodeToString(tokBytes),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(username, gc.Equals, "fred")
}
