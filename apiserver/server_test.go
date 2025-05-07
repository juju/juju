// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/apiserver/errors"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	jujuhttp "github.com/juju/juju/internal/http"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var fastDialOpts = api.DialOpts{}

func dialWebsocketFromURL(c *tc.C, server string, header http.Header) (*websocket.Conn, *http.Response, error) {
	// TODO(rogpeppe) merge this with the very similar dialWebsocket function.
	if header == nil {
		header = http.Header{}
	}
	header.Set("Origin", "http://localhost/")
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), tc.IsTrue)
	tlsConfig := jujuhttp.SecureTLSConfig()
	tlsConfig.RootCAs = caCerts
	tlsConfig.ServerName = "juju-apiserver"

	dialer := &websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	return dialer.Dial(server, header)
}

type serverSuite struct {
	jujutesting.ApiServerSuite
}

var _ = tc.Suite(&serverSuite{})

func (s *serverSuite) TestStop(c *tc.C) {
	conn, machine := s.OpenAPIAsNewMachine(c, state.JobManageModel)

	_, err := apimachiner.NewClient(conn).Machine(context.Background(), machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)

	_, err = apimachiner.NewClient(conn).Machine(context.Background(), machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)

	err = s.Server.Stop()
	c.Assert(err, tc.ErrorIs, apiserver.ErrAPIServerDying)

	_, err = apimachiner.NewClient(conn).Machine(context.Background(), machine.MachineTag())
	// The client has not necessarily seen the server shutdown yet, so there
	// are multiple possible errors. All we should care about is that there is
	// an error, not what the error actually is.
	c.Assert(err, tc.NotNil)

	// Check it can be stopped twice.
	err = s.Server.Stop()
	c.Assert(err, tc.ErrorIs, apiserver.ErrAPIServerDying)

	// nil Server to prevent connection cleanup during teardown complaining due
	// to connection close errors.
	s.Server = nil
}

func (s *serverSuite) TestAPIServerCanListenOnBothIPv4AndIPv6(c *tc.C) {
	domainServices := s.ControllerDomainServices(c)
	controllerConfig, err := domainServices.ControllerConfig().ControllerConfig(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	st := s.ControllerModel(c).State()

	err = st.SetAPIHostPorts(controllerConfig, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	machine, password := f.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	info := s.ControllerModelApiInfo()
	port := info.Ports()[0]
	portString := fmt.Sprintf("%d", port)

	// Now connect twice - using IPv4 and IPv6 endpoints.
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"

	ipv4Conn, err := api.Open(context.Background(), info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer ipv4Conn.Close()
	c.Assert(ipv4Conn.Addr().String(), tc.Equals, "wss://"+net.JoinHostPort("localhost", portString))
	c.Assert(ipv4Conn.APIHostPorts(), tc.DeepEquals, []network.MachineHostPorts{
		network.NewMachineHostPorts(port, "localhost"),
	})

	_, err = apimachiner.NewClient(ipv4Conn).Machine(context.Background(), machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)

	info.Addrs = []string{net.JoinHostPort("::1", portString)}
	ipv6Conn, err := api.Open(context.Background(), info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer ipv6Conn.Close()
	c.Assert(ipv6Conn.Addr().String(), tc.Equals, "wss://"+net.JoinHostPort("::1", portString))
	c.Assert(ipv6Conn.APIHostPorts(), tc.DeepEquals, []network.MachineHostPorts{
		network.NewMachineHostPorts(port, "::1"),
	})

	_, err = apimachiner.NewClient(ipv6Conn).Machine(context.Background(), machine.MachineTag())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serverSuite) TestOpenAsMachineErrors(c *tc.C) {
	assertNotProvisioned := func(err error) {
		c.Assert(err, tc.NotNil)
		c.Assert(err, tc.Satisfies, params.IsCodeNotProvisioned)
		c.Assert(err, tc.ErrorMatches, `machine \d+ not provisioned \(not provisioned\)`)
	}

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	machine, password := f.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	// This does almost exactly the same as OpenAPIAsMachine but checks
	// for failures instead.
	info := s.ControllerModelApiInfo()
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "invalid-nonce"
	st, err := api.Open(context.Background(), info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, tc.IsNil)

	// Try with empty nonce as well.
	info.Nonce = ""
	st, err = api.Open(context.Background(), info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, tc.IsNil)

	// Finally, with the correct one succeeds.
	info.Nonce = "fake_nonce"
	st, err = api.Open(context.Background(), info, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st, tc.NotNil)
	st.Close()

	// Now add another machine, intentionally unprovisioned.
	st1 := s.ControllerModel(c).State()
	stm1, err := st1.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)
	err = stm1.SetPassword(password)
	c.Assert(err, tc.ErrorIsNil)

	// Try connecting, it will fail.
	info.Tag = stm1.Tag()
	info.Nonce = ""
	st, err = api.Open(context.Background(), info, fastDialOpts)
	assertNotProvisioned(err)
	c.Assert(st, tc.IsNil)
}

func dialWebsocket(c *tc.C, addr, path string) (*websocket.Conn, error) {
	// TODO(rogpeppe) merge this with the very similar dialWebsocketFromURL function.
	url := fmt.Sprintf("wss://%s%s", addr, path)
	header := make(http.Header)
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), tc.IsTrue)
	tlsConfig := jujuhttp.SecureTLSConfig()
	tlsConfig.RootCAs = caCerts
	tlsConfig.ServerName = "anything"

	dialer := &websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	conn, _, err := dialer.Dial(url, header)
	return conn, err
}

func (s *serverSuite) TestNonCompatiblePathsAre404(c *tc.C) {
	// We expose the API at '/api', '/' (controller-only), and at '/ModelUUID/api'
	// for the correct location, but other paths should fail.
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)

	// We have to use 'localhost' because that is what the TLS cert says.
	info := s.ControllerModelApiInfo()
	addr := fmt.Sprintf("localhost:%d", info.Ports()[0])

	// '/api' should be fine
	conn, err := dialWebsocket(c, addr, "/api")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/`' should be fine
	conn, err = dialWebsocket(c, addr, "/")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/model/MODELUUID/api' should be fine
	conn, err = dialWebsocket(c, addr, "/model/deadbeef-1234-5678-0123-0123456789ab/api")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/randompath' is not ok
	conn, err = dialWebsocket(c, addr, "/randompath")
	// Unfortunately gorilla/websocket just returns bad handshake, it doesn't
	// give us any information (whether this was a 404 Not Found, Internal
	// Server Error, 200 OK, etc.)
	c.Assert(err, tc.ErrorMatches, `websocket: bad handshake`)
	c.Assert(conn, tc.IsNil)
}

type fakeResource struct {
	stopped bool
}

func (r *fakeResource) Kill() {
	r.stopped = true
}

func (r *fakeResource) Wait() error {
	return nil
}

func (s *serverSuite) bootstrapHasPermissionTest(c *tc.C) (state.Entity, names.ControllerTag) {
	uTag := names.NewUserTag("foobar")

	accessService := s.ControllerDomainServices(c).Access()
	userUUID, _, err := accessService.AddUser(context.Background(), service.AddUserArg{
		Name:        user.NameFromTag(uTag),
		DisplayName: "Foo Bar",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("password")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	user, err := accessService.GetUser(context.Background(), userUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := s.ControllerModel(c).State()
	cTag := names.NewControllerTag(st.ControllerUUID())

	return authentication.TaggedUser(user, uTag), cTag
}

func (s *serverSuite) TestAPIHandlerHasPermissionLogin(c *tc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)

	domainServices := s.ControllerDomainServices(c)

	st := s.ControllerModel(c).State()
	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.StatePool(), st, domainServices, u)
	defer handler.Kill()

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, ctag, false)

}

func (s *serverSuite) TestAPIHandlerHasPermissionSuperUser(c *tc.C) {
	u, ctag := s.bootstrapHasPermissionTest(c)
	domainServices := s.ControllerDomainServices(c)

	handler, _ := apiserver.TestingAPIHandlerWithEntity(c, s.StatePool(), s.ControllerModel(c).State(), domainServices, u)
	defer handler.Kill()

	err := domainServices.Access().UpdatePermission(context.Background(), access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
		Change:  permission.Grant,
		Subject: usertesting.GenNewName(c, u.Tag().Id()),
	})
	c.Assert(err, tc.ErrorIsNil)

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, ctag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, ctag, true)
}

func (s *serverSuite) TestAPIHandlerHasPermissionLoginToken(c *tc.C) {
	user := names.NewUserTag("fred")
	token, err := apitesting.NewJWT(apitesting.JWTParams{
		Controller: coretesting.ControllerTag.Id(),
		User:       user.String(),
		Access: map[string]any{
			coretesting.ControllerTag.String(): "superuser",
			coretesting.ModelTag.String():      "write",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	domainServices := s.ControllerDomainServices(c)

	delegator := &jwt.PermissionDelegator{Token: token}
	st := s.ControllerModel(c).State()
	handler, _ := apiserver.TestingAPIHandlerWithToken(c, s.StatePool(), st, domainServices, token, delegator)
	defer handler.Kill()

	apiserver.AssertHasPermission(c, handler, permission.LoginAccess, coretesting.ControllerTag, true)
	apiserver.AssertHasPermission(c, handler, permission.SuperuserAccess, coretesting.ControllerTag, true)
	apiserver.AssertHasPermission(c, handler, permission.WriteAccess, coretesting.ModelTag, true)
}

func (s *serverSuite) TestAPIHandlerMissingPermissionLoginToken(c *tc.C) {
	user := names.NewUserTag("fred")
	token, err := apitesting.NewJWT(apitesting.JWTParams{
		Controller: coretesting.ControllerTag.Id(),
		User:       user.String(),
		Access: map[string]any{
			coretesting.ControllerTag.String(): "superuser",
			coretesting.ModelTag.String():      "write",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	domainServices := s.ControllerDomainServices(c)

	delegator := &jwt.PermissionDelegator{token}
	st := s.ControllerModel(c).State()
	handler, _ := apiserver.TestingAPIHandlerWithToken(c, s.StatePool(), st, domainServices, token, delegator)
	defer handler.Kill()
	err = handler.HasPermission(context.Background(), permission.AdminAccess, coretesting.ModelTag)
	var reqError *errors.AccessRequiredError
	c.Assert(jujuerrors.As(err, &reqError), tc.IsTrue)
	c.Assert(reqError, tc.DeepEquals, &errors.AccessRequiredError{
		RequiredAccess: map[names.Tag]permission.Access{
			coretesting.ModelTag: permission.AdminAccess,
		},
	})
}

func (s *serverSuite) TestAPIHandlerTeardownInitialModel(c *tc.C) {
	s.checkAPIHandlerTeardown(c, s.ControllerModel(c).State())
}

func (s *serverSuite) TestAPIHandlerTeardownOtherModel(c *tc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	modelUUID := testing.GenModelUUID(c)
	name := makeModel(c, s.TxnRunnerFactory(), s.AdminUserUUID, modelUUID, "another-model")

	otherState := f.MakeModel(c, &factory.ModelParams{
		UUID: modelUUID,
		Name: name,
	})
	defer otherState.Close()
	s.checkAPIHandlerTeardown(c, otherState)
}

func (s *serverSuite) TestClosesStateFromPool(c *tc.C) {
	// We need to skip this until we get off the state backed object store.
	// As the object store is a dependency of the apiserver, it is no longer
	// possible to close the state pool when the connection closes. As this
	// is just temporary code, until we move away from the state backed object
	// store, we can skip this test.
	c.Skip("skip until we get off the state backed object store")

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	otherState := f.MakeModel(c, &factory.ModelParams{
		UUID: s.DefaultModelUUID,
	})
	defer otherState.Close()

	// Ensure the model's in the pool but not referenced.
	m, release := s.Model(c, s.DefaultModelUUID.String())
	release()

	// Make a request for the model API to check it releases
	// state back into the pool once the connection is closed.
	info := s.ControllerModelApiInfo()
	addr := fmt.Sprintf("localhost:%d", info.Ports()[0])
	conn, err := dialWebsocket(c, addr, fmt.Sprintf("/model/%s/api", s.DefaultModelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// Don't make an assertion about whether the remove call returns
	// true - that's dependent on whether the server has reacted to
	// the connection being closed yet, so it's racy.
	_, err = s.StatePool().Remove(s.DefaultModelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	assertStateBecomesClosed(c, m.State())
}

func assertStateBecomesClosed(c *tc.C, st *state.State) {
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
	c.Assert(checkModel, tc.PanicMatches, "Session already closed")
}

func (s *serverSuite) checkAPIHandlerTeardown(c *tc.C, st *state.State) {
	domainServices := s.ControllerDomainServices(c)

	handler, resources := apiserver.TestingAPIHandler(c, s.StatePool(), st, domainServices)
	resource := new(fakeResource)
	resources.Register(resource)

	c.Assert(resource.stopped, tc.IsFalse)
	handler.Kill()
	c.Assert(resource.stopped, tc.IsTrue)
}
