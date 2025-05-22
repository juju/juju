// Copyright 2024 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakerytest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	macaroonservice "github.com/juju/juju/domain/macaroon/service"
	macaroonstate "github.com/juju/juju/domain/macaroon/state"
	domaintesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

// NOTE(jack-w-shaw): Integrating these tests with a real DQLite-backed service
// is the least bad option. The difficulty arises from the fact that the intention
// behind these tests is to test the integration of our own macaroon authentication
// code with the 'third-party' (Canonical controlled) go-macaroon-bakery code.
//
// Particularly much of go-macaroon-bakery depends on injecting an implementation
// of an interface defined within the library (dbrootkeystore.ContextBacking). The
// way in which this interface is then used is an implementation detail of the
// macaroon-bakery.
//
// This means that mocks are not fit for purpose, as we end up making assertions
// against third part implementation details. A stateful fake was another option,
// but would be quite complex and error prone. Using our own DQLite-backed macaroon
// root key service is the best option
type macaroonAuthSuite struct {
	domaintesting.ControllerSuite
	discharger              *bakerytest.Discharger
	authenticator           *Authenticator
	clock                   *testclock.Clock
	controllerConfigService *MockControllerConfigService
	accessService           *MockAccessService
	macaroonService         *macaroonservice.Service

	controllerConfig map[string]interface{}
}

func TestMacaroonAuthSuite(t *stdtesting.T) {
	tc.Run(t, &macaroonAuthSuite{})
}

func (s *macaroonAuthSuite) SetUpTest(c *tc.C) {
	s.discharger = bakerytest.NewDischarger(nil)
	s.controllerConfig = map[string]interface{}{
		controller.IdentityURL: s.discharger.Location(),
	}
	s.clock = testclock.NewClock(time.Now())
	s.ControllerSuite.SetUpTest(c)
	s.macaroonService = macaroonservice.NewService(
		macaroonstate.NewState(s.TxnRunnerFactory()),
		s.clock,
	)
	s.macaroonService.InitialiseBakeryConfig(c.Context())
}

func (s *macaroonAuthSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(s.controllerConfig, nil).AnyTimes()

	agentAuthGetter := authentication.NewAgentAuthenticatorGetter(nil, nil, loggertesting.WrapCheckLog(c))

	authenticator, err := NewAuthenticator(
		c.Context(),
		nil,
		model.UUID(testing.ModelTag.Id()),
		s.controllerConfigService,
		nil,
		s.accessService,
		s.macaroonService,
		agentAuthGetter,
		s.clock,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.authenticator = authenticator

	return ctrl
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

func (s *macaroonAuthSuite) TestServerBakery(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	bsvc, err := ServerBakery(c.Context(), s.authenticator, &alwaysIdent{IdentityLocation: discharger.Location()})
	c.Assert(err, tc.IsNil)

	cav := []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  discharger.Location(),
				Condition: "is-authenticated-user",
			},
			"username",
		),
	}
	mac, err := bsvc.Oven.NewMacaroon(c.Context(), bakery.LatestVersion, cav, bakery.NoOp)
	c.Assert(err, tc.IsNil)

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(c.Context(), mac)
	c.Assert(err, tc.ErrorIsNil)

	_, cond, err := bsvc.Oven.VerifyMacaroon(c.Context(), ms)
	c.Assert(err, tc.IsNil)
	c.Assert(cond, tc.DeepEquals, []string{"declared username fred"})
	authChecker := bsvc.Checker.Auth(ms)
	ai, err := authChecker.Allow(c.Context(), identchecker.LoginOp)
	c.Assert(err, tc.IsNil)
	c.Assert(ai.Identity.Id(), tc.Equals, "fred")
}

func (s *macaroonAuthSuite) TestExpiredKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bsvc, err := ServerBakeryExpiresImmediately(c.Context(), s.authenticator, &alwaysIdent{})
	c.Assert(err, tc.IsNil)

	cav := []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Condition: "is-authenticated-user",
			},
			"username",
		),
	}
	mac, err := bsvc.Oven.NewMacaroon(c.Context(), bakery.LatestVersion, cav, bakery.NoOp)
	c.Assert(err, tc.IsNil)

	// Advance time here because the root key is created during NewMacaroon.
	// The clock needs to move over here to expire the root key correctly
	s.clock.Advance(time.Second)

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(c.Context(), mac)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = bsvc.Oven.VerifyMacaroon(c.Context(), ms)
	c.Assert(err, tc.ErrorMatches, "verification failed: macaroon not found in storage")
}
