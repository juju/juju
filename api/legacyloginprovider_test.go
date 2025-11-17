// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type legacyLoginProviderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&legacyLoginProviderSuite{})

// TestLegacyProviderLogin verifies that the legacy login provider
// works for login and returns the password as the token.
func (s *legacyLoginProviderSuite) TestLegacyProviderLogin(c *gc.C) {
	info := s.APIInfo(c)

	username := names.NewUserTag("admin")
	password := jujutesting.AdminSecret

	lp := api.NewLegacyLoginProvider(username, password, "", nil, nil)
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

	lp := api.NewLegacyLoginProvider(nil, password, "", nil, nil)
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
	)
	got, err := lp.AuthHeader()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, header)
}
