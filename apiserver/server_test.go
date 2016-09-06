// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
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

	_, err = apimachiner.NewState(st).Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	err = srv.Stop()
	c.Assert(err, jc.ErrorIsNil)

	_, err = apimachiner.NewState(st).Machine(machine.MachineTag())
	// The client has not necessarily seen the server shutdown yet, so there
	// are multiple possible errors. All we should care about is that there is
	// an error, not what the error actually is.
	c.Assert(err, gc.NotNil)

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

	_, err = apimachiner.NewState(ipv4State).Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	apiInfo.Addrs = []string{net.JoinHostPort("::1", portString)}
	ipv6State, err := api.Open(apiInfo, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer ipv6State.Close()
	c.Assert(ipv6State.Addr(), gc.Equals, net.JoinHostPort("::1", portString))
	c.Assert(ipv6State.APIHostPorts(), jc.DeepEquals, [][]network.HostPort{
		network.NewHostPorts(port, "::1"),
	})

	_, err = apimachiner.NewState(ipv6State).Machine(machine.MachineTag())
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

	dialOpts := mongo.DialOpts{
		Timeout:       5 * time.Second,
		SocketTimeout: 5 * time.Second,
	}
	st, err := state.Open(s.State.ModelTag(), s.State.ControllerTag(), mongoInfo, dialOpts, nil)
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
	defer func() {
		err := st.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	// Make sure the pinger has started.
	s.assertAlive(c, machine, true)
}

func (s *serverSuite) TestUnitLoginStartsPinger(c *gc.C) {
	// Create a new service and unit to verify "agent alive" behavior.
	unit, password := s.Factory.MakeUnitReturningPassword(c, nil)

	// Not alive yet.
	s.assertAlive(c, unit, false)

	// Login as the unit agent of the created unit.
	st := s.OpenAPIAs(c, unit.Tag(), password)
	defer func() {
		err := st.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	// Make sure the pinger has started.
	s.assertAlive(c, unit, true)
}

func (s *serverSuite) assertAlive(c *gc.C, entity presence.Agent, expectAlive bool) {
	s.State.StartSync()
	alive, err := entity.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, gc.Equals, expectAlive)
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
	config.TlsConfig = utils.SecureTLSConfig()
	if tlsVersion > 0 {
		// This is for testing only. Please don't muck with the maxtlsversion in
		// production.
		config.TlsConfig.MaxVersion = tlsVersion
	}
	config.TlsConfig.RootCAs = pool
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
	// We expose the API at '/api', '/' (controller-only), and at '/ModelUUID/api'
	// for the correct location, but other paths should fail.
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	srv := newServer(c, s.State)
	defer srv.Stop()

	// We have to use 'localhost' because that is what the TLS cert says.
	addr := fmt.Sprintf("localhost:%d", srv.Addr().Port)

	// '/api' should be fine
	conn, err := dialWebsocket(c, addr, "/api", 0)
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()

	// '/`' should be fine
	conn, err = dialWebsocket(c, addr, "/", 0)
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
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.IdentityURL: s.discharger.Location(),
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

	err = bsvc.(*bakery.Service).Check(ms, checkers.New())
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
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.IdentityURL:       s.discharger.Location(),
		controller.IdentityPublicKey: wrongKey.Public.String(),
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

func (s *serverSuite) bootstrapHasPermissionTest(c *gc.C) (*state.User, names.ControllerTag) {
	u, err := s.State.AddUser("foobar", "Foo Bar", "password", "read")
	c.Assert(err, jc.ErrorIsNil)
	user := u.UserTag()

	ctag, err := names.ParseControllerTag("controller-" + s.State.ControllerUUID())
	c.Assert(err, jc.ErrorIsNil)
	cu, err := s.State.UserAccess(user, ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cu.Access, gc.Equals, description.LoginAccess)
	return u, ctag
}

func (s *serverSuite) TestAPIHandlerHasPermissionLogin(c *gc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.State, s.State, u)
	defer handler.Kill()

	apiserver.AssertHasPermission(c, handler, description.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, description.AddModelAccess, ctag, false)
	apiserver.AssertHasPermission(c, handler, description.SuperuserAccess, ctag, false)
}

func (s *serverSuite) TestAPIHandlerHasPermissionAdmodel(c *gc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)
	user := u.UserTag()

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.State, s.State, u)
	defer handler.Kill()

	ua, err := s.State.SetUserAccess(user, ctag, description.AddModelAccess)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ua.Access, gc.Equals, description.AddModelAccess)

	apiserver.AssertHasPermission(c, handler, description.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, description.AddModelAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, description.SuperuserAccess, ctag, false)
}

func (s *serverSuite) TestAPIHandlerHasPermissionSuperUser(c *gc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)
	user := u.UserTag()

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.State, s.State, u)
	defer handler.Kill()

	ua, err := s.State.SetUserAccess(user, ctag, description.SuperuserAccess)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ua.Access, gc.Equals, description.SuperuserAccess)

	apiserver.AssertHasPermission(c, handler, description.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, description.AddModelAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, description.SuperuserAccess, ctag, true)
}

func (s *serverSuite) TestAPIHandlerTeardownInitialEnviron(c *gc.C) {
	s.checkAPIHandlerTeardown(c, s.State, s.State)
}

func (s *serverSuite) TestAPIHandlerTeardownOtherEnviron(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	s.checkAPIHandlerTeardown(c, s.State, otherState)
}

func (s *serverSuite) TestAPIHandlerConnectedModel(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	handler, _ := apiserver.TestingAPIHandler(c, s.State, otherState)
	c.Check(handler.ConnectedModel(), gc.Equals, otherState.ModelUUID())
}

func (s *serverSuite) checkAPIHandlerTeardown(c *gc.C, srvSt, st *state.State) {
	handler, resources := apiserver.TestingAPIHandler(c, srvSt, st)
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
		Cert:        []byte(coretesting.ServerCert),
		Key:         []byte(coretesting.ServerKey),
		Tag:         names.NewMachineTag("0"),
		LogDir:      c.MkDir(),
		NewObserver: func() observer.Observer { return &fakeobserver.Instance{} },
	})
	c.Assert(err, jc.ErrorIsNil)
	return srv
}
