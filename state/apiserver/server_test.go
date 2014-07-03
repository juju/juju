// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	stdtesting "testing"
	"time"

	"code.google.com/p/go.net/websocket"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cert"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/state/presence"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var fastDialOpts = api.DialOpts{}

type serverSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) TestStop(c *gc.C) {
	// Start our own instance of the server so we have
	// a handle on it to stop it.
	srv, err := apiserver.NewServer(s.State, apiserver.ServerConfig{
		Port: 0,
		Cert: []byte(coretesting.ServerCert),
		Key:  []byte(coretesting.ServerKey),
	})
	c.Assert(err, gc.IsNil)
	defer srv.Stop()

	stm, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = stm.SetPassword(password)
	c.Assert(err, gc.IsNil)

	// Note we can't use openAs because we're not connecting to
	// s.APIConn.
	apiInfo := &api.Info{
		Tag:      stm.Tag().String(),
		Password: password,
		Nonce:    "fake_nonce",
		Addrs:    []string{srv.Addr()},
		CACert:   coretesting.CACert,
	}
	st, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer st.Close()

	_, err = st.Machiner().Machine(stm.Tag().String())
	c.Assert(err, gc.IsNil)

	err = srv.Stop()
	c.Assert(err, gc.IsNil)

	_, err = st.Machiner().Machine(stm.Tag().String())
	// The client has not necessarily seen the server shutdown yet,
	// so there are two possible errors.
	if err != rpc.ErrShutdown && err != io.ErrUnexpectedEOF {
		c.Fatalf("unexpected error from request: %v", err)
	}

	// Check it can be stopped twice.
	err = srv.Stop()
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestAPIServerCanListenOnBothIPv4AndIPv6(c *gc.C) {
	// Start our own instance of the server listening on
	// both IPv4 and IPv6 localhost addresses and port 54321.
	srv, err := apiserver.NewServer(s.State, apiserver.ServerConfig{
		Port: 54321,
		Cert: []byte(coretesting.ServerCert),
		Key:  []byte(coretesting.ServerKey),
	})
	c.Assert(err, gc.IsNil)
	defer srv.Stop()

	// srv.Addr() always reports "localhost" as address.
	// This way it can be used to construct URLs which
	// will work for both IPv4 and IPv6-only networks,
	// as localhost resolves as both 127.0.0.1 and ::1.
	c.Assert(srv.Addr(), gc.Equals, "localhost:54321")

	stm, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = stm.SetPassword(password)
	c.Assert(err, gc.IsNil)

	// Now connect twice - using IPv4 and IPv6 endpoints.
	apiInfo := &api.Info{
		Tag:      stm.Tag().String(),
		Password: password,
		Nonce:    "fake_nonce",
		Addrs:    []string{net.JoinHostPort("127.0.0.1", "54321")},
		CACert:   coretesting.CACert,
	}
	ipv4State, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer ipv4State.Close()
	c.Assert(ipv4State.Addr(), gc.Equals, "127.0.0.1:54321")
	c.Assert(ipv4State.APIHostPorts(), jc.DeepEquals, [][]network.HostPort{
		[]network.HostPort{{network.NewAddress("127.0.0.1", network.ScopeMachineLocal), 54321}},
	})

	_, err = ipv4State.Machiner().Machine(stm.Tag().String())
	c.Assert(err, gc.IsNil)

	apiInfo.Addrs = []string{net.JoinHostPort("::1", "54321")}
	ipv6State, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer ipv6State.Close()
	c.Assert(ipv6State.Addr(), gc.Equals, "[::1]:54321")
	c.Assert(ipv6State.APIHostPorts(), jc.DeepEquals, [][]network.HostPort{
		[]network.HostPort{{network.NewAddress("::1", network.ScopeMachineLocal), 54321}},
	})

	_, err = ipv6State.Machiner().Machine(stm.Tag().String())
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestOpenAsMachineErrors(c *gc.C) {
	assertNotProvisioned := func(err error) {
		c.Assert(err, gc.NotNil)
		c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
		c.Assert(err, gc.ErrorMatches, `machine \d+ is not provisioned`)
	}
	stm, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = stm.SetPassword(password)
	c.Assert(err, gc.IsNil)

	// This does almost exactly the same as OpenAPIAsMachine but checks
	// for failures instead.
	_, info, err := s.APIConn.Environ.StateInfo()
	info.Tag = stm.Tag().String()
	info.Password = password
	info.Nonce = "invalid-nonce"
	st, err := api.Open(info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, gc.IsNil)

	// Try with empty nonce as well.
	info.Nonce = ""
	st, err = api.Open(info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, gc.IsNil)

	// Finally, with the correct one succeeds.
	info.Nonce = "fake_nonce"
	st, err = api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.NotNil)
	st.Close()

	// Now add another machine, intentionally unprovisioned.
	stm1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = stm1.SetPassword(password)
	c.Assert(err, gc.IsNil)

	// Try connecting, it will fail.
	info.Tag = stm1.Tag().String()
	info.Nonce = ""
	st, err = api.Open(info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, gc.IsNil)
}

func (s *serverSuite) TestMachineLoginStartsPinger(c *gc.C) {
	// This is the same steps as OpenAPIAsNewMachine but we need to assert
	// the agent is not alive before we actually open the API.
	// Create a new machine to verify "agent alive" behavior.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)

	// Not alive yet.
	s.assertAlive(c, machine, false)

	// Login as the machine agent of the created machine.
	st := s.OpenAPIAsMachine(c, machine.Tag().String(), password, "fake_nonce")

	// Make sure the pinger has started.
	s.assertAlive(c, machine, true)

	// Now make sure it stops when connection is closed.
	c.Assert(st.Close(), gc.IsNil)

	// Sync, then wait for a bit to make sure the state is updated.
	s.State.StartSync()
	<-time.After(coretesting.ShortWait)
	s.State.StartSync()

	s.assertAlive(c, machine, false)
}

func (s *serverSuite) TestUnitLoginStartsPinger(c *gc.C) {
	// Create a new service and unit to verify "agent alive" behavior.
	service := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)

	// Not alive yet.
	s.assertAlive(c, unit, false)

	// Login as the unit agent of the created unit.
	st := s.OpenAPIAs(c, unit.Tag().String(), password)

	// Make sure the pinger has started.
	s.assertAlive(c, unit, true)

	// Now make sure it stops when connection is closed.
	c.Assert(st.Close(), gc.IsNil)

	// Sync, then wait for a bit to make sure the state is updated.
	s.State.StartSync()
	<-time.After(coretesting.ShortWait)
	s.State.StartSync()

	s.assertAlive(c, unit, false)
}

func (s *serverSuite) assertAlive(c *gc.C, entity presence.Presencer, isAlive bool) {
	s.State.StartSync()
	alive, err := entity.AgentPresence()
	c.Assert(err, gc.IsNil)
	c.Assert(alive, gc.Equals, isAlive)
}

func dialWebsocket(c *gc.C, addr, path string) (*websocket.Conn, error) {
	origin := "http://localhost/"
	url := fmt.Sprintf("wss://%s%s", addr, path)
	config, err := websocket.NewConfig(url, origin)
	c.Assert(err, gc.IsNil)
	pool := x509.NewCertPool()
	xcert, err := cert.ParseCert(coretesting.CACert)
	c.Assert(err, gc.IsNil)
	pool.AddCert(xcert)
	config.TlsConfig = &tls.Config{RootCAs: pool}
	return websocket.DialConfig(config)
}

func (s *serverSuite) TestNonCompatiblePathsAre404(c *gc.C) {
	// we expose the API at '/' for compatibility, and at '/ENVUUID/api'
	// for the correct location, but other Paths should fail.
	srv, err := apiserver.NewServer(s.State, apiserver.ServerConfig{
		Port: 0,
		Cert: []byte(coretesting.ServerCert),
		Key:  []byte(coretesting.ServerKey),
	})
	c.Assert(err, gc.IsNil)
	defer srv.Stop()
	// We have to use 'localhost' because that is what the TLS cert says.
	// So find just the Port for the server
	_, portString, err := net.SplitHostPort(srv.Addr())
	c.Assert(err, gc.IsNil)
	addr := "localhost:" + portString
	// '/' should be fine
	conn, err := dialWebsocket(c, addr, "/")
	c.Assert(err, gc.IsNil)
	conn.Close()
	// '/environment/ENVIRONUUID/api' should be fine
	conn, err = dialWebsocket(c, addr, "/environment/dead-beef-123456/api")
	c.Assert(err, gc.IsNil)
	conn.Close()

	// '/randompath' is not ok
	conn, err = dialWebsocket(c, addr, "/randompath")
	// Unfortunately go.net/websocket just returns Bad Status, it doesn't
	// give us any information (whether this was a 404 Not Found, Internal
	// Server Error, 200 OK, etc.)
	c.Assert(err, gc.ErrorMatches, `websocket.Dial wss://localhost:\d+/randompath: bad status`)
	c.Assert(conn, gc.IsNil)
}
