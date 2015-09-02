// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net"
	"net/http"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type loginV2Suite struct {
	loginSuite
}

type loginV2MacaroonSuite struct {
	jujutesting.JujuConnSuite
	discharger *bakerytest.Discharger
	username   string
}

func (s *loginV2MacaroonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.discharger = bakerytest.NewDischarger(nil, s.Checker)
}

func (s *loginV2MacaroonSuite) TearDownTest(c *gc.C) {
	if s.discharger != nil {
		s.discharger.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *loginV2MacaroonSuite) Checker(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
	return []checkers.Caveat{checkers.DeclaredCaveat("username", s.username)}, nil
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
var _ = gc.Suite(&loginV2MacaroonSuite{})

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
	c.Assert(user.LastLogin(), gc.NotNil)
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

func (s *loginV2MacaroonSuite) newServer(c *gc.C) *apiserver.Server {
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(s.State, listener, apiserver.ServerConfig{
		Cert: []byte(coretesting.ServerCert),
		Key:  []byte(coretesting.ServerKey),
		Tag:  names.NewMachineTag("0"),
	})
	c.Assert(err, jc.ErrorIsNil)
	return srv
}
