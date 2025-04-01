// Copyright 2024 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

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
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&macaroonAuthSuite{})

func (s *macaroonAuthSuite) SetUpTest(c *gc.C) {
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
	s.macaroonService.InitialiseBakeryConfig(context.Background())
}

func (s *macaroonAuthSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(s.controllerConfig, nil).AnyTimes()

	agentAuthGetter := authentication.NewAgentAuthenticatorGetter(nil, nil, loggertesting.WrapCheckLog(c))

	authenticator, err := NewAuthenticator(
		context.Background(),
		nil,
		model.UUID(testing.ModelTag.Id()),
		s.controllerConfigService,
		nil,
		s.accessService,
		s.macaroonService,
		agentAuthGetter,
		s.clock,
	)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *macaroonAuthSuite) TestServerBakery(c *gc.C) {
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

	bsvc, err := ServerBakery(context.Background(), s.authenticator, &alwaysIdent{IdentityLocation: discharger.Location()})
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
	defer s.setupMocks(c).Finish()

	bsvc, err := ServerBakeryExpiresImmediately(context.Background(), s.authenticator, &alwaysIdent{})
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

	// Advance time here because the root key is created during NewMacaroon.
	// The clock needs to move over here to expire the root key correctly
	s.clock.Advance(time.Second)

	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(context.Background(), mac)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = bsvc.Oven.VerifyMacaroon(context.Background(), ms)
	c.Assert(err, gc.ErrorMatches, "verification failed: macaroon not found in storage")
}
