// Copyright 2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"context"
	"net/http"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2-unstable/bakerytest"
	bakery2 "gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/controller"
	statetesting "github.com/juju/juju/state/testing"
)

// TODO(babbageclunk): These have been extracted pretty mechanically
// from the API server tests as part of the apiserver/httpserver
// split. They should be updated to test via the public interface
// rather than the export_test functions.

type macaroonCommonSuite struct {
	statetesting.StateSuite
	discharger    *bakerytest.Discharger
	authenticator *stateauthenticator.Authenticator
}

func (s *macaroonCommonSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	authenticator, err := stateauthenticator.NewAuthenticator(s.StatePool, clock.WallClock)
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
	s.discharger = bakerytest.NewDischarger(nil, noCheck)
	s.ControllerConfig = map[string]interface{}{
		controller.IdentityURL: s.discharger.Location(),
	}
	s.macaroonCommonSuite.SetUpTest(c)
}

func (s *macaroonAuthSuite) TestServerBakery(c *gc.C) {
	m, err := stateauthenticator.ServerMacaroon(s.authenticator)
	c.Assert(err, gc.IsNil)
	bsvc, err := stateauthenticator.ServerBakeryService(s.authenticator)
	c.Assert(err, gc.IsNil)

	// Check that we can add a third party caveat addressed to the
	// discharger, which indirectly ensures that the discharger's public
	// key has been added to the bakery service's locator.
	m = m.Clone()
	err = bsvc.AddCaveat(m, checkers.Caveat{
		Location:  s.discharger.Location(),
		Condition: "true",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can discharge the macaroon and check it with
	// the service.
	client := httpbakery.NewClient()
	mac, err := bakery2.NewLegacyMacaroon(m)
	c.Assert(err, jc.ErrorIsNil)
	ms, err := client.DischargeAll(context.Background(), mac)
	c.Assert(err, jc.ErrorIsNil)

	err = bsvc.(*bakery.Service).Check(ms, checkers.New())
	c.Assert(err, gc.IsNil)
}

func noCheck(_ *http.Request, _, _ string) ([]checkers.Caveat, error) {
	return nil, nil
}

type macaroonAuthWrongPublicKeySuite struct {
	macaroonCommonSuite
}

var _ = gc.Suite(&macaroonAuthWrongPublicKeySuite{})

func (s *macaroonAuthWrongPublicKeySuite) SetUpTest(c *gc.C) {
	s.discharger = bakerytest.NewDischarger(nil, noCheck)
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
	m, err := stateauthenticator.ServerMacaroon(s.authenticator)
	c.Assert(err, gc.IsNil)
	m = m.Clone()
	bsvc, err := stateauthenticator.ServerBakeryService(s.authenticator)
	c.Assert(err, gc.IsNil)
	err = bsvc.AddCaveat(m, checkers.Caveat{
		Location:  s.discharger.Location(),
		Condition: "true",
	})
	c.Assert(err, gc.IsNil)
	client := httpbakery.NewClient()

	mac, err := bakery2.NewLegacyMacaroon(m)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.DischargeAll(context.Background(), mac)
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from ".*": third party refused discharge: cannot discharge: discharger cannot decode caveat id: public key mismatch`)
}

type macaroonNoURLSuite struct {
	macaroonCommonSuite
}

var _ = gc.Suite(&macaroonNoURLSuite{})

func (s *macaroonNoURLSuite) TestNoBakeryWhenNoIdentityURL(c *gc.C) {
	// By default, when there is no identity location, no
	// bakery service or macaroon is created.
	_, err := stateauthenticator.ServerMacaroon(s.authenticator)
	c.Assert(err, gc.ErrorMatches, "macaroon authentication is not configured")
	_, err = stateauthenticator.ServerBakeryService(s.authenticator)
	c.Assert(err, gc.ErrorMatches, "macaroon authentication is not configured")
}
