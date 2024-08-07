// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujuhttp "github.com/juju/juju/internal/http"
	coretesting "github.com/juju/juju/internal/testing"
	jtesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type legacyLoginProviderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&legacyLoginProviderSuite{})

func (s *legacyLoginProviderSuite) APIInfo() *api.Info {
	srv := apiservertesting.NewAPIServer(func(modelUUID string) (interface{}, error) {
		var err error
		if modelUUID != "" && modelUUID != jtesting.ModelTag.Id() {
			err = fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
		}
		return &testRootAPI{}, err
	})
	s.AddCleanup(func(_ *gc.C) { srv.Close() })
	info := &api.Info{
		Addrs:          srv.Addrs,
		CACert:         jtesting.CACert,
		ControllerUUID: jtesting.ControllerTag.Id(),
		ModelTag:       jtesting.ModelTag,
	}
	return info
}

// TestLegacyProviderLogin verifies that the legacy login provider
// works for login and returns the password as the token.
func (s *legacyLoginProviderSuite) TestLegacyProviderLogin(c *gc.C) {
	info := s.APIInfo()

	username := names.NewUserTag("admin")
	password := jujutesting.AdminSecret

	lp := api.NewLegacyLoginProvider(username, password, "", nil, nil, nil)
	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()
	c.Check(err, jc.ErrorIsNil)
}

func (s *legacyLoginProviderSuite) TestLegacyProviderWithNilTag(c *gc.C) {
	info := s.APIInfo(c)
	password := jujutesting.AdminSecret

	lp := api.NewLegacyLoginProvider(nil, password, "", nil, nil, nil)
	_, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, gc.ErrorMatches, `failed to authenticate request: unauthorized \(unauthorized access\)`)
}

// A separate suite for tests that don't need to connect to a controller.
type legacyLoginProviderBasicSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&legacyLoginProviderBasicSuite{})

func (s *legacyLoginProviderBasicSuite) TestLegacyProviderAuthHeader(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, header)
}

func (s *legacyLoginProviderBasicSuite) TestLegacyProviderAuthHeaderWithNilTag(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, header)
}
