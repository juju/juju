// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	jujutesting "github.com/juju/juju/juju/testing"
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
	c.Assert(lp.Token(), gc.Equals, password)
}
