// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/juju/api"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/testing/factory"
)

type loginV2Suite struct {
	loginSuite
}

var _ = gc.Suite(&loginV2Suite{
	loginSuite{
		baseLoginSuite{
			setAdminApi: func(srv *apiserver.Server) {
				apiserver.SetAdminApiVersions(srv, 2)
			},
		},
	},
})

func (s *loginV2Suite) TestClientLoginToEnvironment(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info := s.APIInfo(c)
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	client := apiState.Client()
	_, err = client.GetEnvironmentConstraints()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginV2Suite) TestClientLoginToServer(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info := s.APIInfo(c)
	info.EnvironTag = names.EnvironTag{}
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	client := apiState.Client()
	_, err = client.GetEnvironmentConstraints()
	c.Assert(err, gc.ErrorMatches, `logged in to server, no environment, "Client" not supported`)
}

func (s *loginV2Suite) TestClientLoginToServerNoAccessToStateServerEnv(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		NoEnvUser: true,
		Password:  password,
	})

	info := s.APIInfo(c)
	info.Tag = user.Tag()
	info.Password = password
	info.EnvironTag = names.EnvironTag{}
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

func (s *loginV2Suite) TestClientLoginToRootOldClient(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info := s.APIInfo(c)
	info.EnvironTag = names.EnvironTag{}
	apiState, err := api.OpenWithVersion(info, api.DialOpts{}, 1)
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	client := apiState.Client()
	_, err = client.GetEnvironmentConstraints()
	c.Assert(err, jc.ErrorIsNil)
}
