// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/azure/internal/azureauth"
)

type TokenResourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TokenResourceSuite{})

func (s *TokenResourceSuite) TestTokenResource(c *gc.C) {
	out := azureauth.TokenResource("https://graph.windows.net")
	c.Assert(out, gc.Equals, "https://graph.windows.net/")
	out = azureauth.TokenResource("https://graph.windows.net/")
	c.Assert(out, gc.Equals, "https://graph.windows.net/")
}
