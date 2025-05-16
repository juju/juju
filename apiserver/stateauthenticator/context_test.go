// Copyright 2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	stdtesting "testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakerytest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

// TODO(babbageclunk): These have been extracted pretty mechanically
// from the API server tests as part of the apiserver/httpserver
// split. They should be updated to test via the public interface
// rather than the export_test functions.

type macaroonCommonSuite struct {
	discharger              *bakerytest.Discharger
	authenticator           *Authenticator
	clock                   *testclock.Clock
	controllerConfigService *MockControllerConfigService
	accessService           *MockAccessService
	macaroonService         *MockMacaroonService

	controllerConfig map[string]interface{}
}

func (s *macaroonCommonSuite) SetUpTest(c *tc.C) {
	s.clock = testclock.NewClock(time.Now())
}

func (s *macaroonCommonSuite) TearDownTest(c *tc.C) {
	if s.discharger != nil {
		s.discharger.Close()
	}
}

func (s *macaroonCommonSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(s.controllerConfig, nil).AnyTimes()

	s.macaroonService = NewMockMacaroonService(ctrl)
	s.macaroonService.EXPECT().GetLocalUsersKey(gomock.Any()).Return(bakery.MustGenerateKey(), nil).MinTimes(1)
	s.macaroonService.EXPECT().GetLocalUsersThirdPartyKey(gomock.Any()).Return(bakery.MustGenerateKey(), nil).MinTimes(1)
	s.macaroonService.EXPECT().GetExternalUsersThirdPartyKey(gomock.Any()).Return(bakery.MustGenerateKey(), nil).AnyTimes()

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

type macaroonAuthWrongPublicKeySuite struct {
	macaroonCommonSuite
}

func TestMacaroonAuthWrongPublicKeySuite(t *stdtesting.T) {
	tc.Run(t, &macaroonAuthWrongPublicKeySuite{})
}
func (s *macaroonAuthWrongPublicKeySuite) SetUpTest(c *tc.C) {
	s.discharger = bakerytest.NewDischarger(nil)
	wrongKey, err := bakery.GenerateKey()
	c.Assert(err, tc.IsNil)
	s.controllerConfig = map[string]interface{}{
		controller.IdentityURL:       s.discharger.Location(),
		controller.IdentityPublicKey: wrongKey.Public.String(),
	}
	s.macaroonCommonSuite.SetUpTest(c)
}

func (s *macaroonAuthWrongPublicKeySuite) TearDownTest(c *tc.C) {
	s.discharger.Close()
}

func (s *macaroonAuthWrongPublicKeySuite) TestDischargeFailsWithWrongPublicKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ctx := c.Context()
	client := httpbakery.NewClient()

	m, err := macaroon.New(nil, nil, "loc", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	mac, err := bakery.NewLegacyMacaroon(m)
	c.Assert(err, tc.ErrorIsNil)
	cav := checkers.Caveat{
		Location:  s.discharger.Location(),
		Condition: "true",
	}
	anotherKey, err := bakery.GenerateKey()
	c.Assert(err, tc.ErrorIsNil)
	loc := bakery.NewThirdPartyStore()
	loc.AddInfo(s.discharger.Location(), bakery.ThirdPartyInfo{})
	err = mac.AddCaveat(ctx, cav, anotherKey, loc)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.DischargeAll(ctx, mac)
	c.Assert(err, tc.ErrorMatches, `cannot get discharge from ".*": third party refused discharge: cannot discharge: discharger cannot decode caveat id: public key mismatch`)
}

type macaroonNoURLSuite struct {
	macaroonCommonSuite
}

func TestMacaroonNoURLSuite(t *stdtesting.T) { tc.Run(t, &macaroonNoURLSuite{}) }
func (s *macaroonNoURLSuite) TestNoBakeryWhenNoIdentityURL(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// By default, when there is no identity location, no bakery is created.
	_, err := ServerBakery(c.Context(), s.authenticator, nil)
	c.Assert(err, tc.ErrorMatches, "macaroon authentication is not configured")
}
