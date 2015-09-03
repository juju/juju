// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	jujutesting.JujuConnSuite
	discharger *bakerytest.Discharger
	checker    func(string, string) ([]checkers.Caveat, error)
	srv        *apiserver.Server
	client     api.Connection
}

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.discharger = bakerytest.NewDischarger(nil, func(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
		return s.checker(cond, arg)
	})

	environTag := names.NewEnvironTag(s.State.EnvironUUID())

	// Make a new version of the state that doesn't object to us
	// changing the identity URL, so we can create a state server
	// that will see that.
	st, err := state.Open(environTag, s.MongoInfo(c), mongo.DefaultDialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = st.UpdateEnvironConfig(map[string]interface{}{
		config.IdentityURL: s.discharger.Location(),
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.client, s.srv = s.newServer(c)

	s.Factory.MakeUser(c, &factory.UserParams{
		Name: "test",
	})
}

func (s *macaroonLoginSuite) TearDownTest(c *gc.C) {
	s.srv.Stop()

	s.discharger.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *macaroonLoginSuite) TestSuccessfulLogin(c *gc.C) {
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		if cond == "is-authenticated-user" {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "test")}, nil
		}
		return nil, errors.New("unknown caveat")
	}
	err := s.client.Login(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestFailedToObtainDischargeLogin(c *gc.C) {
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		return nil, errors.New("unknown caveat")
	}
	err := s.client.Login(nil, "", "")
	c.Assert(err, gc.ErrorMatches, "failed to obtain the macaroon discharge.*cannot discharge: unknown caveat")
}

func (s *macaroonLoginSuite) TestUnknownUserLogin(c *gc.C) {
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		if cond == "is-authenticated-user" {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "testUnknown")}, nil
		}
		return nil, errors.New("unknown caveat")
	}
	err := s.client.Login(nil, "", "")
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")
}

// newServer returns a new running API server.
func (s *macaroonLoginSuite) newServer(c *gc.C) (api.Connection, *apiserver.Server) {
	listener, err := net.Listen("tcp", "localhost:0")
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(s.State, listener, apiserver.ServerConfig{
		Cert: []byte(coretesting.ServerCert),
		Key:  []byte(coretesting.ServerKey),
		Tag:  names.NewMachineTag("0"),
	})
	c.Assert(err, jc.ErrorIsNil)

	client, err := api.Open(&api.Info{
		Addrs:  []string{srv.Addr().String()},
		CACert: coretesting.CACert,
	}, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	return client, srv
}
