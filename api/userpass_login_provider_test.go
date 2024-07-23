// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"

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

type userPassLoginProviderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&userPassLoginProviderSuite{})

// TestUserPassLogin verifies that the username and password login provider
// works for login and returns the password as the token.
func (s *userPassLoginProviderSuite) TestUserPassLogin(c *gc.C) {
	info := s.APIInfo(c)

	username := names.NewUserTag("admin")
	password := jujutesting.AdminSecret

	lp := api.NewUserpassLoginProvider(username, password, "", nil, nil, nil)
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

// A separate suite for tests that don't need to connect to a controller.
type userPassLoginProviderBasicSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&userPassLoginProviderBasicSuite{})

func (s *userPassLoginProviderBasicSuite) TestUserPassAuthHeader(c *gc.C) {
	userTag := names.NewUserTag("bob")
	password := "test-password"
	nonce := "test-nonce"
	header := jujuhttp.BasicAuthHeader(userTag.String(), password)
	header.Add(params.MachineNonceHeader, nonce)
	header.Add(httpbakery.BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))
	lp := api.NewUserpassLoginProvider(
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
