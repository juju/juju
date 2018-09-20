// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/x509"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testserver"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
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

func (s *serverSuite) SetUpTest(c *gc.C) {
	// Tests here check that pingers are started. We need to inject
	// the feature flags into the controller very early.
	s.ControllerConfigAttrs = map[string]interface{}{
		"features": []string{feature.OldPresence},
	}
	s.JujuConnSuite.SetUpTest(c)
}

func (s *serverSuite) TestStop(c *gc.C) {
	// Start our own instance of the server so we have
	// a handle on it to stop it.
	srv := testserver.NewServer(c, s.StatePool)
	defer assertStop(c, srv)

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// Note we can't use openAs because we're not connecting to
	info := srv.Info
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	info.ModelTag = s.Model.ModelTag()

	st, err := api.Open(info, fastDialOpts)
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
	srv := testserver.NewServer(c, s.StatePool)
	defer assertStop(c, srv)

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	info := srv.Info
	port := info.Ports()[0]
	portString := fmt.Sprintf("%d", port)

	// Now connect twice - using IPv4 and IPv6 endpoints.
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	info.ModelTag = s.Model.ModelTag()

	ipv4State, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer ipv4State.Close()
	c.Assert(ipv4State.Addr(), gc.Equals, net.JoinHostPort("localhost", portString))
	c.Assert(ipv4State.APIHostPorts(), jc.DeepEquals, [][]network.HostPort{
		network.NewHostPorts(port, "localhost"),
	})

	_, err = apimachiner.NewState(ipv4State).Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	info.Addrs = []string{net.JoinHostPort("::1", portString)}
	ipv6State, err := api.Open(info, fastDialOpts)
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
	// Create a new application and unit to verify "agent alive" behavior.
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

func dialWebsocket(c *gc.C, addr, path string) (*websocket.Conn, error) {
	// TODO(rogpeppe) merge this with the very similar dialWebsocketFromURL function.
	url := fmt.Sprintf("wss://%s%s", addr, path)
	header := make(http.Header)
	header.Set("Origin", "http://localhost/")
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), jc.IsTrue)
	tlsConfig := utils.SecureTLSConfig()
	tlsConfig.RootCAs = caCerts
	tlsConfig.ServerName = "anything"

	dialer := &websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	conn, _, err := dialer.Dial(url, header)
	return conn, err
}

func (s *serverSuite) TestNonCompatiblePathsAre404(c *gc.C) {
	// We expose the API at '/api', '/' (controller-only), and at '/ModelUUID/api'
	// for the correct location, but other paths should fail.
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	srv := testserver.NewServer(c, s.StatePool)
	defer assertStop(c, srv)

	// We have to use 'localhost' because that is what the TLS cert says.
	addr := fmt.Sprintf("localhost:%d", srv.Info.Ports()[0])

	// '/api' should be fine
	conn, err := dialWebsocket(c, addr, "/api")
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()

	// '/`' should be fine
	conn, err = dialWebsocket(c, addr, "/")
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()

	// '/model/MODELUUID/api' should be fine
	conn, err = dialWebsocket(c, addr, "/model/deadbeef-1234-5678-0123-0123456789ab/api")
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()

	// '/randompath' is not ok
	conn, err = dialWebsocket(c, addr, "/randompath")
	// Unfortunately gorilla/websocket just returns bad handshake, it doesn't
	// give us any information (whether this was a 404 Not Found, Internal
	// Server Error, 200 OK, etc.)
	c.Assert(err, gc.ErrorMatches, `websocket: bad handshake`)
	c.Assert(conn, gc.IsNil)
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
	access, err := s.State.UserPermission(user, ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.LoginAccess)
	return u, ctag
}

func (s *serverSuite) TestAPIHandlerHasPermissionLogin(c *gc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.StatePool, s.State, u)
	defer handler.Kill()

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, ctag, false)
}

func (s *serverSuite) TestAPIHandlerHasPermissionSuperUser(c *gc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)
	user := u.UserTag()

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.StatePool, s.State, u)
	defer handler.Kill()

	ua, err := s.State.SetUserAccess(user, ctag, permission.SuperuserAccess)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ua.Access, gc.Equals, permission.SuperuserAccess)

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, ctag, true)
}

func (s *serverSuite) TestAPIHandlerTeardownInitialModel(c *gc.C) {
	s.checkAPIHandlerTeardown(c, s.State, s.State)
}

func (s *serverSuite) TestAPIHandlerTeardownOtherModel(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	s.checkAPIHandlerTeardown(c, s.State, otherState)
}

func (s *serverSuite) TestAPIHandlerConnectedModel(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	handler, _ := apiserver.TestingAPIHandler(c, s.StatePool, otherState)
	defer handler.Kill()
	c.Check(handler.ConnectedModel(), gc.Equals, otherState.ModelUUID())
}

func (s *serverSuite) TestClosesStateFromPool(c *gc.C) {
	cfg := testserver.DefaultServerConfig(c)
	server := testserver.NewServerWithConfig(c, s.StatePool, cfg)
	defer assertStop(c, server)

	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()

	// Ensure the model's in the pool but not referenced.
	st, err := s.StatePool.Get(otherState.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	st.Release()

	// Make a request for the model API to check it releases
	// state back into the pool once the connection is closed.
	addr := fmt.Sprintf("localhost:%d", server.Info.Ports()[0])
	conn, err := dialWebsocket(c, addr, fmt.Sprintf("/model/%s/api", st.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()

	// Don't make an assertion about whether the remove call returns
	// true - that's dependent on whether the server has reacted to
	// the connection being closed yet, so it's racy.
	_, err = s.StatePool.Remove(otherState.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	assertStateBecomesClosed(c, st.State)
}

func assertStateBecomesClosed(c *gc.C, st *state.State) {
	// This is gross but I can't see any other way to check for
	// closedness outside the state package.
	checkModel := func() {
		attempt := utils.AttemptStrategy{
			Total: coretesting.LongWait,
			Delay: coretesting.ShortWait,
		}
		for a := attempt.Start(); a.Next(); {
			// This will panic once the state is closed.
			_, _ = st.Model()
		}
		// If we got here then st is still open.
		st.Close()
	}
	c.Assert(checkModel, gc.PanicMatches, "Session already closed")
}

func (s *serverSuite) checkAPIHandlerTeardown(c *gc.C, srvSt, st *state.State) {
	handler, resources := apiserver.TestingAPIHandler(c, s.StatePool, st)
	resource := new(fakeResource)
	resources.Register(resource)

	c.Assert(resource.stopped, jc.IsFalse)
	handler.Kill()
	c.Assert(resource.stopped, jc.IsTrue)
}

type stopper interface {
	Stop() error
}

func assertStop(c *gc.C, stopper stopper) {
	c.Assert(stopper.Stop(), gc.IsNil)
}

type mockAuthenticator struct {
}

func (a *mockAuthenticator) Authenticate(req *http.Request) (httpcontext.AuthInfo, error) {
	return httpcontext.AuthInfo{}, nil
}

func (a *mockAuthenticator) AuthenticateLoginRequest(
	serverHost string,
	modelUUID string,
	req params.LoginRequest,
) (httpcontext.AuthInfo, error) {
	tag, err := names.ParseTag(req.AuthTag)
	if err != nil {
		return httpcontext.AuthInfo{}, errors.Trace(err)
	}
	return httpcontext.AuthInfo{
		Entity: &mockEntity{tag: tag},
	}, nil
}

func (a *mockAuthenticator) CreateLocalLoginMacaroon(tag names.UserTag) (*macaroon.Macaroon, error) {
	return nil, nil
}

type mockEntity struct {
	tag names.Tag
}

func (e *mockEntity) Tag() names.Tag {
	return e.tag
}
