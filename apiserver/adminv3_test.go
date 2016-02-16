// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testing/factory"
)

type loginV3Suite struct {
	loginSuite
}

var _ = gc.Suite(&loginV3Suite{
	loginSuite{
		baseLoginSuite{
			setAdminApi: func(srv *apiserver.Server) {
				apiserver.SetAdminApiVersions(srv, 3)
			},
		},
	},
})

func (s *loginV3Suite) TestClientLoginToEnvironment(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info := s.APIInfo(c)
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	client := apiState.Client()
	_, err = client.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginV3Suite) TestClientLoginToServer(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info := s.APIInfo(c)
	info.ModelTag = names.ModelTag{}
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	client := apiState.Client()
	_, err = client.GetModelConstraints()
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `logged in to server, no model, "Client" not supported`,
		Code:    "not supported",
	})
}

func (s *loginV3Suite) TestClientLoginToServerNoAccessToControllerEnv(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		NoModelUser: true,
		Password:    password,
	})

	info := s.APIInfo(c)
	info.Tag = user.Tag()
	info.Password = password
	info.ModelTag = names.ModelTag{}
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()
	// The user now has last login updated.
	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	lastLogin, err := user.LastLogin()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lastLogin, gc.NotNil)
}

func (s *loginV3Suite) TestClientLoginToRootOldClient(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info := s.APIInfo(c)
	info.ModelTag = names.ModelTag{}
	_, err := api.OpenWithVersion(info, api.DialOpts{}, 2)
	c.Assert(err, gc.ErrorMatches, ".*this version of Juju does not support login from old clients.*")
}
