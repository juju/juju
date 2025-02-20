// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth_test

import (
	"github.com/gliderlabs/ssh"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jujussh "github.com/juju/utils/v3/ssh"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/sshserver/auth"
)

type authorizerSuite struct {
	statetesting.StateSuite

	authorizer auth.Authenticator
}

var _ = gc.Suite(&authorizerSuite{})

func (s *authorizerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	var err error
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	logger := loggo.GetLogger("authorizerSuite")
	s.authorizer, err = auth.NewAuthenticator(s.StatePool, logger)
	c.Assert(err, gc.IsNil)
}

func (s *authorizerSuite) TestAuthorizedKeysPerUser(c *gc.C) {
	cfgM1, err := s.Model.Config()
	c.Assert(err, gc.IsNil)
	authKeysM1 := jujussh.SplitAuthorisedKeys(cfgM1.AuthorizedKeys())
	c.Assert(authKeysM1, gc.HasLen, 1)
	defaultPublicKeyM1, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authKeysM1[0]))
	c.Assert(err, gc.IsNil)
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "bob",
			NoModelUser: true,
		},
	)
	_, err = s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: s.Owner,
			Access:    permission.ReadAccess,
		},
	)
	c.Assert(err, gc.IsNil)

	testCases := []struct {
		name            string
		userTag         names.UserTag
		userKey         ssh.PublicKey
		expectedSuccess bool
	}{
		{
			name:            "test for owner of both models",
			userTag:         s.Model.Owner(),
			userKey:         defaultPublicKeyM1,
			expectedSuccess: true,
		},
		{
			name:            "test for owner of no model",
			userTag:         names.NewUserTag("nomodel"),
			userKey:         defaultPublicKeyM1,
			expectedSuccess: false,
		},
		{
			name:            "test for user with read access to a single model",
			userTag:         user.UserTag(),
			userKey:         defaultPublicKeyM1,
			expectedSuccess: true,
		},
	}

	for _, tc := range testCases {
		c.Logf("test: %s", tc.name)
		success := s.authorizer.PublicKeyAuthentication(tc.userTag, tc.userKey)
		c.Assert(err, gc.IsNil)
		c.Assert(success, gc.Equals, tc.expectedSuccess)
	}
}
