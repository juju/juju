// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/presence"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var fastDialOpts = api.DialOpts{}

type serverSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) TestStop(c *gc.C) {
	// Start our own instance of the server so we have
	// a handle on it to stop it.
	srv := newServer(c, s.State)
	defer srv.Stop()

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// A net.TCPAddr cannot be directly stringified into a valid hostname.
	address := fmt.Sprintf("localhost:%d", srv.Addr().Port)

	// Note we can't use openAs because we're not connecting to
	apiInfo := &api.Info{
		Tag:      machine.Tag(),
		Password: password,
		Nonce:    "fake_nonce",
		Addrs:    []string{address},
		CACert:   coretesting.CACert,
		ModelTag: s.State.ModelTag(),
	}
	st, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	_, err = st.Machiner().Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	err = srv.Stop()
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.Machiner().Machine(machine.MachineTag())
	err = errors.Cause(err)
	// The client has not necessarily seen the server shutdown yet,
	// so there are two possible errors.
	if err != rpc.ErrShutdown && err != io.ErrUnexpectedEOF {
		c.Fatalf("unexpected error from request: %#v, expected rpc.ErrShutdown or io.ErrUnexpectedEOF", err)
	}

	// Check it can be stopped twice.
	err = srv.Stop()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestAPIServerCanListenOnBothIPv4AndIPv6(c *gc.C) {
	err := s.State.SetAPIHostPorts(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Start our own instance of the server listening on
	// both IPv4 and IPv6 localhost addresses and an ephemeral port.
	srv := newServer(c, s.State)
	defer srv.Stop()

	port := srv.Addr().Port
	portString := fmt.Sprintf("%d", port)

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// Now connect twice - using IPv4 and IPv6 endpoints.
	apiInfo := &api.Info{
		Tag:      machine.Tag(),
		Password: password,
		Nonce:    "fake_nonce",
		Addrs:    []string{net.JoinHostPort("127.0.0.1", portString)},
		CACert:   coretesting.CACert,
		ModelTag: s.State.ModelTag(),
	}
	ipv4State, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer ipv4State.Close()
	c.Assert(ipv4State.Addr(), gc.Equals, net.JoinHostPort("127.0.0.1", portString))
	c.Assert(ipv4State.APIHostPorts(), jc.DeepEquals, [][]network.HostPort{
		network.NewHostPorts(port, "127.0.0.1"),
	})

	_, err = ipv4State.Machiner().Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	apiInfo.Addrs = []string{net.JoinHostPort("::1", portString)}
	ipv6State, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer ipv6State.Close()
	c.Assert(ipv6State.Addr(), gc.Equals, net.JoinHostPort("::1", portString))
	c.Assert(ipv6State.APIHostPorts(), jc.DeepEquals, [][]network.HostPort{
		network.NewHostPorts(port, "::1"),
	})

	_, err = ipv6State.Machiner().Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestOpenAsMachineErrors(c *gc.C) {
	assertNotProvisioned := func(err error) {
		c.Assert(err, gc.NotNil)
		c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
		c.Assert(err, gc.ErrorMatches, `machine \d+ not provisioned \(not provisioned\)`)
	}

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// This does almost exactly the same as OpenAPIAsMachine but checks
	// for failures instead.
	info := s.APIInfo(c)
	info.Tag = machine.Tag()
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.NotNil)
	st.Close()

	// Now add another machine, intentionally unprovisioned.
	stm1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = stm1.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	// Try connecting, it will fail.
	info.Tag = stm1.Tag()
	info.Nonce = ""
	st, err = api.Open(info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, gc.IsNil)
}

func (s *serverSuite) TestNewServerDoesNotAccessState(c *gc.C) {
	mongoInfo := s.MongoInfo(c)

	proxy := testing.NewTCPProxy(c, mongoInfo.Addrs[0])
	mongoInfo.Addrs = []string{proxy.Addr()}

	st, err := state.Open(s.State.ModelTag(), mongoInfo, mongo.DefaultDialOpts(), nil)
	c.Assert(err, gc.IsNil)
	defer st.Close()

	// Now close the proxy so that any attempts to use the
	// controller will fail.
	proxy.Close()

	// Creating the server should succeed because it doesn't
	// access the state (note that newServer does not log in,
	// which *would* access the state).
	srv := newServer(c, st)
	srv.Stop()
}

func (s *serverSuite) TestMachineLoginStartsPinger(c *gc.C) {
	// This is the same steps as OpenAPIAsNewMachine but we need to assert
	// the agent is not alive before we actually open the API.
	// Create a new machine to verify "agent alive" behavior.
	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// Not alive yet.
	s.assertAlive(c, machine, false)

	// Login as the machine agent of the created machine.
	st := s.OpenAPIAsMachine(c, machine.Tag(), password, "fake_nonce")

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
	unit, password := s.Factory.MakeUnitReturningPassword(c, nil)

	// Not alive yet.
	s.assertAlive(c, unit, false)

	// Login as the unit agent of the created unit.
	st := s.OpenAPIAs(c, unit.Tag(), password)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, gc.Equals, isAlive)
}

func dialWebsocket(c *gc.C, addr, path string, tlsVersion uint16) (*websocket.Conn, error) {
	origin := "http://localhost/"
	url := fmt.Sprintf("wss://%s%s", addr, path)
	config, err := websocket.NewConfig(url, origin)
	c.Assert(err, jc.ErrorIsNil)
	pool := x509.NewCertPool()
	xcert, err := cert.ParseCert(coretesting.CACert)
	c.Assert(err, jc.ErrorIsNil)
	pool.AddCert(xcert)
	config.TlsConfig = &tls.Config{
		RootCAs:    pool,
		MaxVersion: tlsVersion,
	}
	return websocket.DialConfig(config)
}

func (s *serverSuite) TestMinTLSVersion(c *gc.C) {
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	srv := newServer(c, s.State)
	defer srv.Stop()

	// We have to use 'localhost' because that is what the TLS cert says.
	addr := fmt.Sprintf("localhost:%d", srv.Addr().Port)

	// Specify an unsupported TLS version
	conn, err := dialWebsocket(c, addr, "/", tls.VersionSSL30)
	c.Assert(err, gc.ErrorMatches, ".*protocol version not supported")
	c.Assert(conn, gc.IsNil)
}

func (s *serverSuite) TestNonCompatiblePathsAre404(c *gc.C) {
	// we expose the API at '/' for compatibility, and at '/ModelUUID/api'
	// for the correct location, but other Paths should fail.
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	srv := newServer(c, s.State)
	defer srv.Stop()

	// We have to use 'localhost' because that is what the TLS cert says.
	addr := fmt.Sprintf("localhost:%d", srv.Addr().Port)
	// '/' should be fine
	conn, err := dialWebsocket(c, addr, "/", 0)
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()
	// '/model/MODELUUID/api' should be fine
	conn, err = dialWebsocket(c, addr, "/model/dead-beef-123456/api", 0)
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()

	// '/randompath' is not ok
	conn, err = dialWebsocket(c, addr, "/randompath", 0)
	// Unfortunately go.net/websocket just returns Bad Status, it doesn't
	// give us any information (whether this was a 404 Not Found, Internal
	// Server Error, 200 OK, etc.)
	c.Assert(err, gc.ErrorMatches, `websocket.Dial wss://localhost:\d+/randompath: bad status`)
	c.Assert(conn, gc.IsNil)
}

func (s *serverSuite) TestNoBakeryWhenNoIdentityURL(c *gc.C) {
	srv := newServer(c, s.State)
	defer srv.Stop()
	// By default, when there is no identity location, no
	// bakery service or macaroon is created.
	_, err := apiserver.ServerMacaroon(srv)
	c.Assert(err, gc.ErrorMatches, "macaroon authentication is not configured")
	_, err = apiserver.ServerBakeryService(srv)
	c.Assert(err, gc.ErrorMatches, "macaroon authentication is not configured")
}

type macaroonServerSuite struct {
	jujutesting.JujuConnSuite
	discharger *bakerytest.Discharger
}

var _ = gc.Suite(&macaroonServerSuite{})

func (s *macaroonServerSuite) SetUpTest(c *gc.C) {
	s.discharger = bakerytest.NewDischarger(nil, noCheck)
	s.ConfigAttrs = map[string]interface{}{
		config.IdentityURL: s.discharger.Location(),
	}
	s.JujuConnSuite.SetUpTest(c)
}

func (s *macaroonServerSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *macaroonServerSuite) TestServerBakery(c *gc.C) {
	srv := newServer(c, s.State)
	defer srv.Stop()
	m, err := apiserver.ServerMacaroon(srv)
	c.Assert(err, gc.IsNil)
	bsvc, err := apiserver.ServerBakeryService(srv)
	c.Assert(err, gc.IsNil)

	// Check that we can add a third party caveat addressed to the
	// discharger, which indirectly ensures that the discharger's public
	// key has been added to the bakery service's locator.
	m = m.Clone()
	err = bsvc.AddCaveat(m, checkers.Caveat{
		Location:  s.discharger.Location(),
		Condition: "true",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can discharge the macaroon and check it with
	// the service.
	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(m)
	c.Assert(err, jc.ErrorIsNil)

	err = bsvc.Check(ms, checkers.New())
	c.Assert(err, gc.IsNil)
}

type macaroonServerWrongPublicKeySuite struct {
	jujutesting.JujuConnSuite
	discharger *bakerytest.Discharger
}

var _ = gc.Suite(&macaroonServerWrongPublicKeySuite{})

func (s *macaroonServerWrongPublicKeySuite) SetUpTest(c *gc.C) {
	s.discharger = bakerytest.NewDischarger(nil, noCheck)
	wrongKey, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	s.ConfigAttrs = map[string]interface{}{
		config.IdentityURL:       s.discharger.Location(),
		config.IdentityPublicKey: wrongKey.Public.String(),
	}
	s.JujuConnSuite.SetUpTest(c)
}

func (s *macaroonServerWrongPublicKeySuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *macaroonServerWrongPublicKeySuite) TestDischargeFailsWithWrongPublicKey(c *gc.C) {
	srv := newServer(c, s.State)
	defer srv.Stop()
	m, err := apiserver.ServerMacaroon(srv)
	c.Assert(err, gc.IsNil)
	m = m.Clone()
	bsvc, err := apiserver.ServerBakeryService(srv)
	c.Assert(err, gc.IsNil)
	err = bsvc.AddCaveat(m, checkers.Caveat{
		Location:  s.discharger.Location(),
		Condition: "true",
	})
	c.Assert(err, gc.IsNil)
	client := httpbakery.NewClient()

	_, err = client.DischargeAll(m)
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from ".*": third party refused discharge: cannot discharge: discharger cannot decode caveat id: public key mismatch`)
}

func noCheck(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
	return nil, nil
}

type fakeResource struct {
	stopped bool
}

func (r *fakeResource) Stop() error {
	r.stopped = true
	return nil
}

func (s *serverSuite) TestApiHandlerTeardownInitialEnviron(c *gc.C) {
	s.checkApiHandlerTeardown(c, s.State, s.State)
}

func (s *serverSuite) TestApiHandlerTeardownOtherEnviron(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	s.checkApiHandlerTeardown(c, s.State, otherState)
}

func (s *serverSuite) checkApiHandlerTeardown(c *gc.C, srvSt, st *state.State) {
	handler, resources := apiserver.TestingApiHandler(c, srvSt, st)
	resource := new(fakeResource)
	resources.Register(resource)

	c.Assert(resource.stopped, jc.IsFalse)
	handler.Kill()
	c.Assert(resource.stopped, jc.IsTrue)
}

// newServer returns a new running API server.
func newServer(c *gc.C, st *state.State) *apiserver.Server {
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(st, listener, apiserver.ServerConfig{
		Cert:   []byte(coretesting.ServerCert),
		Key:    []byte(coretesting.ServerKey),
		Tag:    names.NewMachineTag("0"),
		LogDir: c.MkDir(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return srv
}
