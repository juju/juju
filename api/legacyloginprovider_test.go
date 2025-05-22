// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	"net/http"
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujuversion "github.com/juju/juju/core/version"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type legacyLoginProviderSuite struct {
	testing.BaseSuite

	mockRootAPI  *MockRootAPI
	mockAdminAPI *MockAdminAPI
}

func TestLegacyLoginProviderSuite(t *stdtesting.T) {
	tc.Run(t, &legacyLoginProviderSuite{})
}

//go:generate go run go.uber.org/mock/mockgen -typed -package api_test -destination api_mock_test.go -source legacyloginprovider_test.go RootAPI,AdminAPI

type RootAPI interface {
	Admin(id string) (AdminAPI, error)
}

type AdminAPI interface {
	Login(req params.LoginRequest) (params.LoginResult, error)
}

func (s *legacyLoginProviderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockRootAPI = NewMockRootAPI(ctrl)
	s.mockAdminAPI = NewMockAdminAPI(ctrl)
	s.mockRootAPI.EXPECT().Admin(gomock.Any()).Return(s.mockAdminAPI, nil).AnyTimes()

	return ctrl
}

func (s *legacyLoginProviderSuite) APIInfo() *api.Info {
	srv := apiservertesting.NewAPIServer(func(modelUUID string) (interface{}, error) {
		var err error
		if modelUUID != "" && modelUUID != testing.ModelTag.Id() {
			err = fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
		}
		return s.mockRootAPI, err
	})
	s.AddCleanup(func(_ *tc.C) { srv.Close() })
	info := &api.Info{
		Addrs:          srv.Addrs,
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
		ModelTag:       testing.ModelTag,
	}
	return info
}

// TestLegacyProviderLogin verifies that the legacy login provider
// works for login and returns the password as the token.
func (s *legacyLoginProviderSuite) TestLegacyProviderLogin(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAdminAPI.EXPECT().Login(gomock.Any()).DoAndReturn(func(lr params.LoginRequest) (params.LoginResult, error) {
		mc := tc.NewMultiChecker()
		mc.AddExpr("_.CLIArgs", tc.Ignore)
		c.Check(lr, mc, params.LoginRequest{
			AuthTag:       "user-admin",
			Credentials:   "dummy-secret",
			BakeryVersion: 3,
			ClientVersion: jujuversion.Current.String(),
		})
		return params.LoginResult{
			ControllerTag: testing.ControllerTag.String(),
			ModelTag:      testing.ModelTag.String(),
			Servers:       [][]params.HostPort{},
			ServerVersion: jujuversion.Current.String(),
			PublicDNSName: "somewhere.example.com",
		}, nil
	})

	info := s.APIInfo()

	username := names.NewUserTag("admin")
	password := jujutesting.AdminSecret

	lp := api.NewLegacyLoginProvider(username, password, "", nil, nil, nil)
	apiState, err := api.Open(c.Context(), &api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
		ModelTag:       info.ModelTag,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer apiState.Close()
	c.Check(err, tc.ErrorIsNil)
}

func (s *legacyLoginProviderSuite) TestLegacyProviderWithNilTag(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAdminAPI.EXPECT().Login(gomock.Any()).DoAndReturn(func(lr params.LoginRequest) (params.LoginResult, error) {
		mc := tc.NewMultiChecker()
		mc.AddExpr("_.CLIArgs", tc.Ignore)
		c.Check(lr, mc, params.LoginRequest{
			AuthTag:       "",
			Credentials:   "dummy-secret",
			BakeryVersion: 3,
			ClientVersion: jujuversion.Current.String(),
		})
		return params.LoginResult{}, fmt.Errorf("failed to authenticate request: %w", errors.Unauthorized)
	})

	info := s.APIInfo()
	password := jujutesting.AdminSecret

	lp := api.NewLegacyLoginProvider(nil, password, "", nil, nil, nil)
	_, err := api.Open(c.Context(), &api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
		ModelTag:       info.ModelTag,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, tc.ErrorMatches, `failed to authenticate request: unauthorized \(unauthorized access\)`)
}

// A separate suite for tests that don't need to connect to a controller.
type legacyLoginProviderBasicSuite struct {
	testing.BaseSuite
}

func TestLegacyLoginProviderBasicSuite(t *stdtesting.T) {
	tc.Run(t, &legacyLoginProviderBasicSuite{})
}
func (s *legacyLoginProviderBasicSuite) TestLegacyProviderAuthHeader(c *tc.C) {
	userTag := names.NewUserTag("bob")
	password := "test-password"
	nonce := "test-nonce"
	header := jujuhttp.BasicAuthHeader(userTag.String(), password)
	header.Add(params.MachineNonceHeader, nonce)
	header.Add(httpbakery.BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))
	lp := api.NewLegacyLoginProvider(
		userTag,
		password,
		nonce,
		[]macaroon.Slice{},
		nil,
		nil,
	)
	got, err := lp.AuthHeader()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, header)
}

func (s *legacyLoginProviderBasicSuite) TestLegacyProviderAuthHeaderWithNilTag(c *tc.C) {
	password := "test-password"
	nonce := "test-nonce"
	header := http.Header{}
	header.Add(params.MachineNonceHeader, nonce)
	header.Add(httpbakery.BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))
	lp := api.NewLegacyLoginProvider(
		nil,
		password,
		nonce,
		[]macaroon.Slice{},
		nil,
		nil,
	)
	got, err := lp.AuthHeader()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, header)
}
