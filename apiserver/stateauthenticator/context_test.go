// Copyright 2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakerytest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/v3/apiserver/stateauthenticator"
	"github.com/juju/juju/v3/controller"
	statetesting "github.com/juju/juju/v3/state/testing"
)

// TODO(babbageclunk): These have been extracted pretty mechanically
// from the API server tests as part of the apiserver/httpserver
// split. They should be updated to test via the public interface
// rather than the export_test functions.

type macaroonCommonSuite struct {
	statetesting.StateSuite
	discharger    *bakerytest.Discharger
	authenticator *stateauthenticator.Authenticator
	clock         *testclock.Clock
}

func (s *macaroonCommonSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Now())
	authenticator, err := stateauthenticator.NewAuthenticator(s.StatePool, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticator = authenticator
}

func (s *macaroonCommonSuite) TearDownTest(c *gc.C) {
	if s.discharger != nil {
		s.discharger.Close()
	}
	s.StateSuite.TearDownTest(c)
}

type macaroonAuthSuite struct {
	macaroonCommonSuite
}

var _ = gc.Suite(&macaroonAuthSuite{})

func (s *macaroonAuthSuite) SetUpTest(c *gc.C) {
	s.discharger = bakerytest.NewDischarger(nil)
	s.ControllerConfig = map[string]interface{}{
		controller.IdentityURL: s.discharger.Location(),
	}
	s.macaroonCommonSuite.SetUpTest(c)
}

type alwaysIdent struct {
	IdentityLocation string
}

// IdentityFromContext implements IdentityClient.IdentityFromContext.
func (m *alwaysIdent) IdentityFromContext(ctx context.Context) (identchecker.Identity, []checkers.Caveat, error) {
	return identchecker.SimpleIdentity("fred"), nil, nil
}

func (alwaysIdent) DeclaredIdentity(ctx context.Context, declared map[string]string) (identchecker.Identity, error) {
	return nil, errors.New("not called")
}

func (s *macaroonAuthSuite) TestServerBakery(c *gc.C) {
	// TODO - remove when we use bakeryv2 everywhere
	discharger := bakerytest.NewDischarger(nil)
	defer discharger.Close()
	discharger.CheckerP = httpbakery.ThirdPartyCaveatCheckerPFunc(func(ctx context.Context, p httpbakery.ThirdPartyCaveatCheckerParams) ([]checkers.Caveat, error) {
		if p.Caveat != nil && string(p.Caveat.Condition) == "is-authenticated-user" {
			return []checkers.Caveat{
				checkers.DeclaredCaveat("username", "fred"),
			}, nil
		}
		return nil, errors.New("unexpected caveat")
	})

	bsvc, err := stateauthenticator.ServerBakery(s.authenticator, &alwaysIdent{discharger.Location()})
	c.Assert(err, gc.IsNil)

	cav := []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  discharger.Location(),
				Condition: "is-authenticated-user",
			},
			"username",
		),
	}
	mac, err := bsvc.Oven.NewMacaroon(context.Background(), bakery.LatestVersion, cav, bakery.NoOp)
	c.Assert(err, gc.IsNil)

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(context.Background(), mac)
	c.Assert(err, jc.ErrorIsNil)

	_, cond, err := bsvc.Oven.VerifyMacaroon(context.Background(), ms)
	c.Assert(err, gc.IsNil)
	c.Assert(cond, jc.DeepEquals, []string{"declared username fred"})
	authChecker := bsvc.Checker.Auth(ms)
	ai, err := authChecker.Allow(context.Background(), identchecker.LoginOp)
	c.Assert(err, gc.IsNil)
	c.Assert(ai.Identity.Id(), gc.Equals, "fred")
}

func (s *macaroonAuthSuite) TestExpiredKey(c *gc.C) {
	bsvc, err := stateauthenticator.ServerBakeryExpiresImmediately(s.authenticator, &alwaysIdent{})
	c.Assert(err, gc.IsNil)

	cav := []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Condition: "is-authenticated-user",
			},
			"username",
		),
	}
	mac, err := bsvc.Oven.NewMacaroon(context.Background(), bakery.LatestVersion, cav, bakery.NoOp)
	c.Assert(err, gc.IsNil)

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(context.Background(), mac)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = bsvc.Oven.VerifyMacaroon(context.Background(), ms)
	c.Assert(err, gc.ErrorMatches, "verification failed: macaroon not found in storage")
}

type macaroonAuthWrongPublicKeySuite struct {
	macaroonCommonSuite
}

var _ = gc.Suite(&macaroonAuthWrongPublicKeySuite{})

func (s *macaroonAuthWrongPublicKeySuite) SetUpTest(c *gc.C) {
	s.discharger = bakerytest.NewDischarger(nil)
	wrongKey, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	s.ControllerConfig = map[string]interface{}{
		controller.IdentityURL:       s.discharger.Location(),
		controller.IdentityPublicKey: wrongKey.Public.String(),
	}
	s.macaroonCommonSuite.SetUpTest(c)
}

func (s *macaroonAuthWrongPublicKeySuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.StateSuite.TearDownTest(c)
}

func (s *macaroonAuthWrongPublicKeySuite) TestDischargeFailsWithWrongPublicKey(c *gc.C) {
	ctx := context.Background()
	client := httpbakery.NewClient()

	m, err := macaroon.New(nil, nil, "loc", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	mac, err := bakery.NewLegacyMacaroon(m)
	c.Assert(err, jc.ErrorIsNil)
	cav := checkers.Caveat{
		Location:  s.discharger.Location(),
		Condition: "true",
	}
	anotherKey, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)
	loc := bakery.NewThirdPartyStore()
	loc.AddInfo(s.discharger.Location(), bakery.ThirdPartyInfo{})
	err = mac.AddCaveat(ctx, cav, anotherKey, loc)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.DischargeAll(ctx, mac)
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from ".*": third party refused discharge: cannot discharge: discharger cannot decode caveat id: public key mismatch`)
}

type macaroonNoURLSuite struct {
	macaroonCommonSuite
}

var _ = gc.Suite(&macaroonNoURLSuite{})

func (s *macaroonNoURLSuite) TestNoBakeryWhenNoIdentityURL(c *gc.C) {
	// By default, when there is no identity location, no bakery is created.
	_, err := stateauthenticator.ServerBakery(s.authenticator, nil)
	c.Assert(err, gc.ErrorMatches, "macaroon authentication is not configured")
}
