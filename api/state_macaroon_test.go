// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net"
	"net/http"
	"net/http/cookiejar"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	jujutesting.JujuConnSuite
	discharger   *bakerytest.Discharger
	checker      func(string, string) ([]checkers.Caveat, error)
	srv          *apiserver.Server
	client       api.Connection
	bakeryClient *httpbakery.Client
}

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.discharger = bakerytest.NewDischarger(nil, func(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
		return s.checker(cond, arg)
	})
	s.JujuConnSuite.ConfigAttrs = map[string]interface{}{
		config.IdentityURL: s.discharger.Location(),
	}
	s.JujuConnSuite.SetUpTest(c)

	s.client, s.bakeryClient, s.srv = s.newServer(c)
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
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: unknown caveat`)
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

func (s *macaroonLoginSuite) TestConnectStream(c *gc.C) {
	s.PatchValue(api.WebsocketDialConfig, echoURL(c))

	dischargeCount := 0
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		dischargeCount++
		if cond == "is-authenticated-user" {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "test")}, nil
		}
		return nil, errors.New("unknown caveat")
	}
	// First log into the regular API.
	err := s.client.Login(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dischargeCount, gc.Equals, 1)

	// Then check that ConnectStream works OK and that it doesn't need
	// to discharge again.
	conn, err := s.client.ConnectStream("/path", nil)
	c.Assert(err, gc.IsNil)
	defer conn.Close()
	connectURL := connectURLFromReader(c, conn)
	c.Assert(connectURL.Path, gc.Equals, "/environment/"+s.State.EnvironTag().Id()+"/path")
	c.Assert(dischargeCount, gc.Equals, 1)
}

func (s *macaroonLoginSuite) TestConnectStreamFailedDischarge(c *gc.C) {
	// This is really a test for ConnectStream, but to test ConnectStream's
	// discharge failing logic, we need an actual endpoint to test against,
	// and the debug-log endpoint makes a convenient example.

	var dischargeError error
	s.checker = func(cond, arg string) ([]checkers.Caveat, error) {
		if dischargeError != nil {
			return nil, dischargeError
		}
		if cond == "is-authenticated-user" {
			return []checkers.Caveat{checkers.DeclaredCaveat("username", "test")}, nil
		}
		return nil, errors.New("unknown caveat")
	}
	// First log into the regular API.
	err := s.client.Login(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the discharger won't discharge and try
	// logging in again. We should succeed in getting past
	// authorization because we have the cookies (but
	// the actual debug-log endpoint will return an error
	// because there's no all-machines.log file).
	dischargeError = errors.New("no discharge currently allowed")
	conn, err := s.client.ConnectStream("/log", nil)
	c.Assert(err, gc.ErrorMatches, "cannot open log file: .*")
	c.Assert(conn, gc.Equals, nil)

	// Then delete all the cookies by deleting the cookie jar
	// and try again. The login should fail.
	s.bakeryClient.Client.Jar, _ = cookiejar.New(nil)

	conn, err = s.client.ConnectStream("/log", nil)
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: no discharge currently allowed`)
	c.Assert(conn, gc.Equals, nil)
}

// newServer returns a new running API server.
func (s *macaroonLoginSuite) newServer(c *gc.C) (api.Connection, *httpbakery.Client, *apiserver.Server) {
	listener, err := net.Listen("tcp", "localhost:0")
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(s.State, listener, apiserver.ServerConfig{
		Cert: []byte(coretesting.ServerCert),
		Key:  []byte(coretesting.ServerKey),
		Tag:  names.NewMachineTag("0"),
	})
	c.Assert(err, jc.ErrorIsNil)

	bakeryClient := httpbakery.NewClient()

	client, err := api.Open(&api.Info{
		Addrs:      []string{srv.Addr().String()},
		CACert:     coretesting.CACert,
		EnvironTag: s.State.EnvironTag(),
	}, api.DialOpts{
		BakeryClient: bakeryClient,
	})
	c.Assert(err, jc.ErrorIsNil)

	return client, bakeryClient, srv
}
