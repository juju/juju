// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"code.google.com/p/goauth2/oauth"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	auth Auth
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.auth = Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
}

func (s *BaseSuite) patchNewToken(c *gc.C, expectedAuth Auth, expectedScopes string, token *oauth.Token) {
	if expectedScopes == "" {
		expectedScopes = "https://www.googleapis.com/auth/compute https://www.googleapis.com/auth/devstorage.full_control"
	}
	if token == nil {
		token = &oauth.Token{}
	}
	s.PatchValue(&newToken, func(auth Auth, scopes string) (*oauth.Token, error) {
		c.Check(auth, jc.DeepEquals, expectedAuth)
		c.Check(scopes, gc.Equals, expectedScopes)
		return token, nil
	})
}
