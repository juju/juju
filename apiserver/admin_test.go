// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/machiner"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/controller"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/params"
	servertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/testserver"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type baseLoginSuite struct {
	jujutesting.JujuConnSuite

	mgmtSpace *state.Space
}

func (s *baseLoginSuite) SetUpTest(c *gc.C) {
	if s.ControllerConfigAttrs == nil {
		s.ControllerConfigAttrs = make(map[string]interface{})
	}

	s.JujuConnSuite.SetUpTest(c)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)

	var err error
	s.mgmtSpace, err = s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{corecontroller.JujuManagementSpace: "mgmt01"}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseLoginSuite) newServer(c *gc.C) (*api.Info, *apiserver.Server) {
	return s.newServerWithConfig(c, testserver.DefaultServerConfig(c, nil))
}

func (s *baseLoginSuite) newServerWithConfig(c *gc.C, cfg apiserver.ServerConfig) (*api.Info, *apiserver.Server) {
	cfg.Controller = s.JujuConnSuite.Controller
	server := testserver.NewServerWithConfig(c, s.StatePool, cfg)
	s.AddCleanup(func(c *gc.C) { assertStop(c, server) })
	return server.Info, server.APIServer
}

// loginSuite is built on statetesting.StateSuite not JujuConnSuite.
// It also uses a testing clock, not a wall clock.
type loginSuite struct {
	baseSuite
}

var _ = gc.Suite(&loginSuite{})

func (s *loginSuite) TestLoginWithInvalidTag(c *gc.C) {
	info := s.newServer(c)
	st := s.openAPIWithoutLogin(c, info)

	request := &params.LoginRequest{
		AuthTag:     "bar",
		Credentials: "password",
	}

	var response params.LoginResult
	err := st.APICall("Admin", 3, "", "Login", request, &response)
	c.Assert(err, gc.ErrorMatches, `.*"bar" is not a valid tag.*`)
}

func (s *loginSuite) TestBadLogin(c *gc.C) {
	info := s.newServer(c)

	for i, t := range []struct {
		tag      names.Tag
		password string
		err      error
		code     string
	}{{
		tag:      s.Owner,
		password: "wrong password",
		err: &rpc.RequestError{
			Message: "invalid entity name or password",
			Code:    "unauthorized access",
		},
		code: params.CodeUnauthorized,
	}, {
		tag:      names.NewUserTag("unknown"),
		password: "password",
		err: &rpc.RequestError{
			Message: "invalid entity name or password",
			Code:    "unauthorized access",
		},
		code: params.CodeUnauthorized,
	}} {
		c.Logf("test %d; entity %q; password %q", i, t.tag, t.password)
		func() {
			// Open the API without logging in, so we can perform
			// operations on the connection before calling Login.
			st := s.openAPIWithoutLogin(c, info)

			_, err := apimachiner.NewState(st).Machine(names.NewMachineTag("0"))
			c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
				Message: `unknown object type "Machiner"`,
				Code:    "not implemented",
			})

			// Since these are user login tests, the nonce is empty.
			err = st.Login(t.tag, t.password, "", nil)
			c.Assert(errors.Cause(err), gc.DeepEquals, t.err)
			c.Assert(params.ErrCode(err), gc.Equals, t.code)

			_, err = apimachiner.NewState(st).Machine(names.NewMachineTag("0"))
			c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
				Message: `unknown object type "Machiner"`,
				Code:    "not implemented",
			})
		}()
	}
}

func (s *loginSuite) TestLoginAsDeactivatedUser(c *gc.C) {
	info := s.newServer(c)

	st := s.openAPIWithoutLogin(c, info)
	password := "password"
	u := s.Factory.MakeUser(c, &factory.UserParams{Password: password, Disabled: true})

	_, err := st.Client().Status([]string{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Client"`,
		Code:    "not implemented",
	})

	// Since these are user login tests, the nonce is empty.
	err = st.Login(u.Tag(), password, "", nil)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: fmt.Sprintf("user %q is disabled", u.Tag().Id()),
		Code:    "unauthorized access",
	})

	_, err = st.Client().Status([]string{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Client"`,
		Code:    "not implemented",
	})
}

func (s *loginSuite) TestLoginAsDeletedUser(c *gc.C) {
	info := s.newServer(c)

	st := s.openAPIWithoutLogin(c, info)
	password := "password"
	u := s.Factory.MakeUser(c, &factory.UserParams{Password: password})

	_, err := st.Client().Status([]string{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Client"`,
		Code:    "not implemented",
	})

	err = s.State.RemoveUser(u.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	// Since these are user login tests, the nonce is empty.
	err = st.Login(u.Tag(), password, "", nil)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: fmt.Sprintf("user %q is permanently deleted", u.Tag().Id()),
		Code:    "unauthorized access",
	})

	_, err = st.Client().Status([]string{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Client"`,
		Code:    "not implemented",
	})
}

func (s *loginSuite) setupManagementSpace(c *gc.C) *state.Space {
	mgmtSpace, err := s.State.AddSpace("mgmt01", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{corecontroller.JujuManagementSpace: "mgmt01"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	return mgmtSpace
}

func (s *loginSuite) addController(c *gc.C) (state.ControllerNode, string) {
	node, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = node.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	return node, password
}

func (s *loginSuite) TestControllerAgentLogin(c *gc.C) {
	// The agent login tests also check the management space.
	mgmtSpace := s.setupManagementSpace(c)
	info := s.newServer(c)

	node, password := s.addController(c)
	info.Tag = node.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"

	s.assertAgentLogin(c, info, mgmtSpace)
}

func (s *loginSuite) TestLoginAddressesForAgents(c *gc.C) {
	// The agent login tests also check the management space.
	mgmtSpace := s.setupManagementSpace(c)

	info := s.newServer(c)
	machine := s.infoForNewMachine(c, info)

	s.assertAgentLogin(c, machine, mgmtSpace)
}

func (s *loginSuite) loginHostPorts(
	c *gc.C, info *api.Info,
) (connectedAddr string, hostPorts []network.MachineHostPorts) {
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	return st.Addr(), st.APIHostPorts()
}

func (s *loginSuite) assertAgentLogin(c *gc.C, info *api.Info, mgmtSpace *state.Space) {
	err := s.State.SetAPIHostPorts(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Initially just the address we connect with is returned by the helper
	// because there are no APIHostPorts in state.
	connectedAddr, hostPorts := s.loginHostPorts(c, info)
	connectedAddrHost, connectedAddrPortString, err := net.SplitHostPort(connectedAddr)
	c.Assert(err, jc.ErrorIsNil)

	connectedAddrPort, err := strconv.Atoi(connectedAddrPortString)
	c.Assert(err, jc.ErrorIsNil)

	connectedAddrHostPorts := []network.MachineHostPorts{
		network.NewMachineHostPorts(connectedAddrPort, connectedAddrHost),
	}
	c.Assert(hostPorts, gc.DeepEquals, connectedAddrHostPorts)

	// After storing APIHostPorts in state, Login should return the list
	// filtered for agents along with the address we connected to.
	server1Addresses := network.SpaceAddresses{
		network.NewScopedSpaceAddress("server-1", network.ScopePublic),
		network.NewScopedSpaceAddress("10.0.0.1", network.ScopeCloudLocal),
	}
	server1Addresses[1].SpaceID = mgmtSpace.Id()

	server2Addresses := network.SpaceAddresses{
		network.NewScopedSpaceAddress("::1", network.ScopeMachineLocal),
	}

	err = s.State.SetAPIHostPorts([]network.SpaceHostPorts{
		network.SpaceAddressesWithPort(server1Addresses, 123),
		network.SpaceAddressesWithPort(server2Addresses, 456),
	})
	c.Assert(err, jc.ErrorIsNil)

	_, hostPorts = s.loginHostPorts(c, info)

	// The login method is called with a machine tag, so we expect the
	// first return slice to only have the address in the management space.
	expectedAPIHostPorts := []network.MachineHostPorts{
		{{MachineAddress: server1Addresses[1].MachineAddress, NetPort: 123}},
		{{MachineAddress: server2Addresses[0].MachineAddress, NetPort: 456}},
	}
	// Prepended as before with the connection address.
	expectedAPIHostPorts = append(connectedAddrHostPorts, expectedAPIHostPorts...)
	c.Assert(hostPorts, gc.DeepEquals, expectedAPIHostPorts)
}

func (s *loginSuite) TestLoginAddressesForClients(c *gc.C) {
	mgmtSpace := s.setupManagementSpace(c)

	info := s.newServer(c)
	info = s.infoForNewUser(c, info)

	server1Addresses := network.SpaceAddresses{
		network.NewScopedSpaceAddress("server-1", network.ScopePublic),
		network.NewScopedSpaceAddress("10.0.0.1", network.ScopeCloudLocal),
	}
	server1Addresses[1].SpaceID = mgmtSpace.Id()

	server2Addresses := network.SpaceAddresses{
		network.NewScopedSpaceAddress("::1", network.ScopeMachineLocal),
	}

	newAPIHostPorts := []network.SpaceHostPorts{
		network.SpaceAddressesWithPort(server1Addresses, 123),
		network.SpaceAddressesWithPort(server2Addresses, 456),
	}
	err := s.State.SetAPIHostPorts(newAPIHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	exp := []network.MachineHostPorts{
		{
			{
				MachineAddress: network.NewScopedMachineAddress("server-1", network.ScopePublic),
				NetPort:        123,
			},
			{
				MachineAddress: network.NewScopedMachineAddress("10.0.0.1", network.ScopeCloudLocal),
				NetPort:        123,
			},
		}, {
			{
				MachineAddress: network.NewScopedMachineAddress("::1", network.ScopeMachineLocal),
				NetPort:        456,
			},
		},
	}

	_, hostPorts := s.loginHostPorts(c, info)
	// Ignoring the address used to login, the returned API addresses should not
	// Have management space filtering applied.
	c.Assert(hostPorts[1:], gc.DeepEquals, exp)
}

func (s *loginSuite) setupRateLimiting(c *gc.C) {
	// Instead of the defaults, we'll be explicit in our ratelimit setup.
	err := s.State.UpdateControllerConfig(
		map[string]interface{}{
			corecontroller.AgentRateLimitMax:  1,
			corecontroller.AgentRateLimitRate: time.Second,
		}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) infoForNewMachine(c *gc.C, info *api.Info) *api.Info {
	// Make a copy
	newInfo := *info

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	newInfo.Tag = machine.Tag()
	newInfo.Password = password
	newInfo.Nonce = "fake_nonce"
	return &newInfo
}

func (s *loginSuite) infoForNewUser(c *gc.C, info *api.Info) *api.Info {
	// Make a copy
	newInfo := *info

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})

	newInfo.Tag = user.Tag()
	newInfo.Password = password
	return &newInfo
}

func (s *loginSuite) TestRateLimitAgents(c *gc.C) {
	s.setupRateLimiting(c)

	info := s.newServer(c)
	// First agent connection is fine.
	machine1 := s.infoForNewMachine(c, info)
	conn1, err := api.Open(machine1, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn1.Close()

	// Second machine in the same second gets told to go away and try again.
	machine2 := s.infoForNewMachine(c, info)
	_, err = api.Open(machine2, fastDialOpts)
	c.Assert(err.Error(), gc.Equals, "try again (try again)")

	// If we wait a second and try again, it is fine.
	s.Clock.Advance(time.Second)
	conn2, err := api.Open(machine2, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn2.Close()

	// And the next one is limited.
	machine3 := s.infoForNewMachine(c, info)
	_, err = api.Open(machine3, fastDialOpts)
	c.Assert(err.Error(), gc.Equals, "try again (try again)")
}

func (s *loginSuite) TestRateLimitNotApplicableToUsers(c *gc.C) {
	s.setupRateLimiting(c)
	info := s.newServer(c)

	// First agent connection is fine.
	machine1 := s.infoForNewMachine(c, info)
	conn1, err := api.Open(machine1, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn1.Close()

	// User connections are fine.
	user := s.infoForNewUser(c, info)
	conn2, err := api.Open(user, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn2.Close()

	user2 := s.infoForNewUser(c, info)
	conn3, err := api.Open(user2, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn3.Close()
}

func (s *loginSuite) TestNonModelUserLoginFails(c *gc.C) {
	info := s.newServer(c)
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "dummy-password", NoModelUser: true})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	err := s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	info.Password = "dummy-password"
	info.Tag = user.UserTag()
	_, err = api.Open(info, fastDialOpts)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) TestLoginValidationDuringUpgrade(c *gc.C) {
	s.cfg.UpgradeComplete = func() bool {
		// upgrade is in progress
		return false
	}
	s.testLoginDuringMaintenance(c, func(st api.Connection) {
		var statusResult params.FullStatus
		err := st.APICall("Client", 1, "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, jc.ErrorIsNil)

		err = st.APICall("Client", 1, "", "ModelSet", params.ModelSet{}, nil)
		c.Assert(err, jc.Satisfies, params.IsCodeUpgradeInProgress)
	})
}

func (s *loginSuite) TestLoginWhileRestorePending(c *gc.C) {
	s.cfg.RestoreStatus = func() state.RestoreStatus {
		return state.RestorePending
	}
	s.testLoginDuringMaintenance(c, func(st api.Connection) {
		var statusResult params.FullStatus
		err := st.APICall("Client", 1, "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, jc.ErrorIsNil)

		err = st.APICall("Client", 1, "", "ModelSet", params.ModelSet{}, nil)
		c.Assert(err, gc.ErrorMatches, `juju restore is in progress - functionality is limited to avoid data loss`)
	})
}

func (s *loginSuite) TestLoginWhileRestoreInProgress(c *gc.C) {
	s.cfg.RestoreStatus = func() state.RestoreStatus {
		return state.RestoreInProgress
	}
	s.testLoginDuringMaintenance(c, func(st api.Connection) {
		var statusResult params.FullStatus
		err := st.APICall("Client", 1, "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, gc.ErrorMatches, `juju restore is in progress - API is disabled to prevent data loss`)

		err = st.APICall("Client", 1, "", "ModelSet", params.ModelSet{}, nil)
		c.Assert(err, gc.ErrorMatches, `juju restore is in progress - API is disabled to prevent data loss`)
	})
}

func (s *loginSuite) testLoginDuringMaintenance(c *gc.C, check func(api.Connection)) {
	info := s.newServer(c)

	st := s.openAPIWithoutLogin(c, info)
	err := st.Login(s.Owner, s.AdminPassword, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	check(st)
}

func (s *loginSuite) TestMachineLoginDuringMaintenance(c *gc.C) {
	s.cfg.UpgradeComplete = func() bool {
		// upgrade is in progress
		return false
	}
	info := s.newServer(c)
	machine := s.infoForNewMachine(c, info)
	_, err := api.Open(machine, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, `login for machine \d+ blocked because upgrade is in progress`)
}

func (s *loginSuite) TestControllerMachineLoginDuringMaintenance(c *gc.C) {
	s.cfg.UpgradeComplete = func() bool {
		// upgrade is in progress
		return false
	}
	info := s.newServer(c)

	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "nonce"

	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.Close(), jc.ErrorIsNil)
}

func (s *loginSuite) TestControllerAgentLoginDuringMaintenance(c *gc.C) {
	s.cfg.UpgradeComplete = func() bool {
		// upgrade is in progress
		return false
	}
	info := s.newServer(c)

	node, password := s.addController(c)
	info.Tag = node.Tag()
	info.Password = password

	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.Close(), jc.ErrorIsNil)
}

func (s *loginSuite) TestMigratedModelLogin(c *gc.C) {
	modelOwner := s.Factory.MakeUser(c, &factory.UserParams{
		Password: "secret",
	})
	modelState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
	})
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	controllerTag := names.NewControllerTag(utils.MustNewUUID().String())

	// Migrate the model and delete it from the state
	mig, err := modelState.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag:   controllerTag,
			ControllerAlias: "target",
			Addrs:           []string{"1.2.3.4:5555"},
			CACert:          coretesting.CACert,
			AuthTag:         names.NewUserTag("user2"),
			Password:        "secret",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
	}
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(modelState.RemoveDyingModel(), jc.ErrorIsNil)

	info := s.newServer(c)
	info.ModelTag = model.ModelTag()

	// Attempt to open an API connection to the migrated model as a user
	// that had access to the model before it got migrated. We should still
	// be able to connect to the API but we should get back a Redirect
	// error when we actually try to login.
	info.Tag = modelOwner.Tag()
	info.Password = "secret"
	_, err = api.Open(info, fastDialOpts)
	redirErr, ok := errors.Cause(err).(*api.RedirectError)
	c.Assert(ok, gc.Equals, true)

	nhp := network.NewMachineHostPorts(5555, "1.2.3.4")
	c.Assert(redirErr.Servers, jc.DeepEquals, []network.MachineHostPorts{nhp})
	c.Assert(redirErr.CACert, gc.Equals, coretesting.CACert)
	c.Assert(redirErr.FollowRedirect, gc.Equals, false)
	c.Assert(redirErr.ControllerTag, gc.Equals, controllerTag)
	c.Assert(redirErr.ControllerAlias, gc.Equals, "target")

	// Attempt to open an API connection to the migrated model as a user
	// that had NO access to the model before it got migrated. The server
	// should return a not-authorized error when attempting to log in.
	info.Tag = names.NewUserTag("some-other-user")
	_, err = api.Open(info, fastDialOpts)
	c.Assert(params.ErrCode(errors.Cause(err)), gc.Equals, params.CodeUnauthorized)

	// Attempt to open an API connection to the migrated model as the
	// anonymous user; this should also be allowed on account of CMRs.
	info.Tag = names.NewUserTag(api.AnonymousUsername)
	_, err = api.Open(info, fastDialOpts)
	redirErr, ok = errors.Cause(err).(*api.RedirectError)
	c.Assert(ok, gc.Equals, true)
}

func (s *loginSuite) TestAnonymousModelLogin(c *gc.C) {
	info := s.newServer(c)
	conn := s.openAPIWithoutLogin(c, info)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag: names.NewUserTag(api.AnonymousUsername).String(),
	}
	err := conn.APICall("Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.IsNil)
	c.Assert(result.ControllerTag, gc.Equals, s.State.ControllerTag().String())
	c.Assert(result.ModelTag, gc.Equals, s.Model.ModelTag().String())
	c.Assert(result.Facades, jc.DeepEquals, []params.FacadeVersions{
		{Name: "CrossModelRelations", Versions: []int{1, 2}},
		{Name: "NotifyWatcher", Versions: []int{1}},
		{Name: "OfferStatusWatcher", Versions: []int{1}},
		{Name: "RelationStatusWatcher", Versions: []int{1}},
		{Name: "RelationUnitsWatcher", Versions: []int{1}},
		{Name: "RemoteRelationWatcher", Versions: []int{1}},
		{Name: "StringsWatcher", Versions: []int{1}},
	})
}

func (s *loginSuite) TestAnonymousControllerLogin(c *gc.C) {
	info := s.newServer(c)
	// Zero the model tag so that we log into the controller
	// not the model.
	info.ModelTag = names.ModelTag{}
	conn := s.openAPIWithoutLogin(c, info)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag: names.NewUserTag(api.AnonymousUsername).String(),
	}
	err := conn.APICall("Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.IsNil)
	c.Assert(result.ControllerTag, gc.Equals, s.State.ControllerTag().String())
	c.Assert(result.Facades, jc.DeepEquals, []params.FacadeVersions{
		{Name: "CrossController", Versions: []int{1}},
		{Name: "NotifyWatcher", Versions: []int{1}},
	})
}

func (s *loginSuite) TestControllerModel(c *gc.C) {
	info := s.newServer(c)
	st := s.openAPIWithoutLogin(c, info)

	adminUser := s.Owner
	err := st.Login(adminUser, s.AdminPassword, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertRemoteModel(c, st, s.Model.ModelTag())
}

func (s *loginSuite) TestControllerModelBadCreds(c *gc.C) {
	info := s.newServer(c)
	st := s.openAPIWithoutLogin(c, info)

	adminUser := s.Owner
	err := st.Login(adminUser, "bad-password", "", nil)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) TestNonExistentModel(c *gc.C) {
	info := s.newServer(c)

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = names.NewModelTag(uuid.String())
	st := s.openAPIWithoutLogin(c, info)

	adminUser := s.Owner
	err = st.Login(adminUser, s.AdminPassword, "", nil)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: fmt.Sprintf("unknown model: %q", uuid),
		Code:    "model not found",
	})
}

func (s *loginSuite) TestInvalidModel(c *gc.C) {
	info := s.newServer(c)
	info.ModelTag = names.NewModelTag("rubbish")
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, `unable to connect to API: invalid model UUID "rubbish" \(Bad Request\)`)
	c.Assert(st, gc.IsNil)
}

func (s *loginSuite) ensureCachedModel(c *gc.C, uuid string) {
	timeout := time.After(testing.LongWait)
	retry := time.After(0)
	for {
		s.WaitForModelWatchersIdle(c, uuid)
		select {
		case <-retry:
			_, err := s.controller.Model(uuid)
			if err == nil {
				return
			}
			if !errors.IsNotFound(err) {
				c.Fatalf("problem getting model from cache: %v", err)
			}
			retry = time.After(testing.ShortWait)
		case <-timeout:
			c.Fatalf("model %v not seen in cache after %v", uuid, testing.LongWait)
		}
	}
}

func (s *loginSuite) TestOtherModel(c *gc.C) {
	info := s.newServer(c)

	modelOwner := s.Factory.MakeUser(c, nil)
	modelState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
	})
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = model.ModelTag()

	// Ensure that the model has been added to the cache before
	// we try to log in. Otherwise we get stuck waiting for the model
	// to exist, and time isn't moving as we have a test clock.
	s.ensureCachedModel(c, model.UUID())

	st := s.openAPIWithoutLogin(c, info)

	err = st.Login(modelOwner.UserTag(), "password", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertRemoteModel(c, st, model.ModelTag())
}

func (s *loginSuite) TestMachineLoginOtherModel(c *gc.C) {
	// User credentials are checked against a global user list.
	// Machine credentials are checked against model specific
	// machines, so this makes sure that the credential checking is
	// using the correct state connection.
	info := s.newServer(c)

	modelOwner := s.Factory.MakeUser(c, nil)
	modelState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
		ConfigAttrs: map[string]interface{}{
			"controller": false,
		},
	})
	defer modelState.Close()

	f2 := factory.NewFactory(modelState, s.StatePool)
	machine, password := f2.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "test-nonce",
	})

	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the model has been added to the cache before
	// we try to log in. Otherwise we get stuck waiting for the model
	// to exist, and time isn't moving as we have a test clock.
	s.ensureCachedModel(c, model.UUID())

	info.ModelTag = model.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	err = st.Login(machine.Tag(), password, "test-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestMachineLoginOtherModelNotProvisioned(c *gc.C) {
	info := s.newServer(c)

	modelOwner := s.Factory.MakeUser(c, nil)
	modelState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
		ConfigAttrs: map[string]interface{}{
			"controller": false,
		},
	})
	defer modelState.Close()

	f2 := factory.NewFactory(modelState, s.StatePool)
	machine, password := f2.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})

	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the model has been added to the cache before
	// we try to log in. Otherwise we get stuck waiting for the model
	// to exist, and time isn't moving as we have a test clock.
	s.ensureCachedModel(c, model.UUID())

	info.ModelTag = model.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	// If the agent attempts Login before the provisioner has recorded
	// the machine's nonce in state, then the agent should get back an
	// error with code "not provisioned".
	err = st.Login(machine.Tag(), password, "nonce", nil)
	c.Assert(err, gc.ErrorMatches, `machine 0 not provisioned \(not provisioned\)`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
}

func (s *loginSuite) TestOtherModelFromController(c *gc.C) {
	info := s.newServer(c)

	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})

	modelState := s.Factory.MakeModel(c, nil)
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the model has been added to the cache before
	// we try to log in. Otherwise we get stuck waiting for the model
	// to exist, and time isn't moving as we have a test clock.
	s.ensureCachedModel(c, model.UUID())

	info.ModelTag = model.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	err = st.Login(machine.Tag(), password, "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherModelFromControllerOtherNotProvisioned(c *gc.C) {
	info := s.newServer(c)

	managerMachine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})

	// Create a hosted model with an unprovisioned machine that has the
	// same tag as the manager machine.
	hostedModelState := s.Factory.MakeModel(c, nil)
	defer hostedModelState.Close()
	f2 := factory.NewFactory(hostedModelState, s.StatePool)
	workloadMachine, _ := f2.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})
	c.Assert(managerMachine.Tag(), gc.Equals, workloadMachine.Tag())

	hostedModel, err := hostedModelState.Model()
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the model has been added to the cache before
	// we try to log in. Otherwise we get stuck waiting for the model
	// to exist, and time isn't moving as we have a test clock.
	s.ensureCachedModel(c, hostedModel.UUID())

	info.ModelTag = hostedModel.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	// The fact that the machine with the same tag in the hosted
	// model is unprovisioned should not cause the login to fail
	// with "not provisioned", because the passwords don't match.
	err = st.Login(managerMachine.Tag(), password, "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherModelWhenNotController(c *gc.C) {
	info := s.newServer(c)

	machine, password := s.Factory.MakeMachineReturningPassword(c, nil)

	modelState := s.Factory.MakeModel(c, nil)
	defer modelState.Close()

	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the model has been added to the cache before
	// we try to log in. Otherwise we get stuck waiting for the model
	// to exist, and time isn't moving as we have a test clock.
	s.ensureCachedModel(c, model.UUID())

	info.ModelTag = model.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	err = st.Login(machine.Tag(), password, "nonce", nil)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) loginLocalUser(c *gc.C, info *api.Info) (*state.User, params.LoginResult) {
	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})
	conn := s.openAPIWithoutLogin(c, info)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:     user.Tag().String(),
		Credentials: password,
	}
	err := conn.APICall("Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	return user, result
}

func (s *loginSuite) TestLoginResultLocalUser(c *gc.C) {
	info := s.newServer(c)

	user, result := s.loginLocalUser(c, info)
	c.Check(result.UserInfo.Identity, gc.Equals, user.Tag().String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "admin")
}

func (s *loginSuite) TestLoginResultLocalUserEveryoneCreateOnlyNonLocal(c *gc.C) {
	info := s.newServer(c)

	setEveryoneAccess(c, s.State, s.Owner, permission.SuperuserAccess)

	user, result := s.loginLocalUser(c, info)
	c.Check(result.UserInfo.Identity, gc.Equals, user.Tag().String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "admin")
}

func (s *loginSuite) assertRemoteModel(c *gc.C, api api.Connection, expected names.ModelTag) {
	// Look at what the api thinks it has.
	tag, ok := api.ModelTag()
	c.Assert(ok, jc.IsTrue)
	c.Assert(tag, gc.Equals, expected)
	// Look at what the api Client thinks it has.
	client := api.Client()

	// ModelUUID looks at the model tag on the api state connection.
	uuid, ok := client.ModelUUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, expected.Id())

	// The code below is to verify that the API connection is operating on
	// the expected model. We make a change in state on that model, and
	// then check that it is picked up by a call to the API.

	st, err := s.StatePool.Get(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()

	expectedCons := constraints.MustParse("mem=8G")
	err = st.SetModelConstraints(expectedCons)
	c.Assert(err, jc.ErrorIsNil)

	cons, err := client.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, expectedCons)
}

func (s *loginSuite) TestLoginUpdatesLastLoginAndConnection(c *gc.C) {
	info := s.newServer(c)

	now := s.Clock.Now().UTC().Round(time.Second)

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})

	info.Tag = user.Tag()
	info.Password = password
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	// The user now has last login updated.
	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	lastLogin, err := user.LastLogin()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lastLogin, gc.Equals, now)

	// The model user is also updated.
	modelUser, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when, gc.Equals, now)
}

func (s *loginSuite) TestLoginAddsAuditConversationEventually(c *gc.C) {
	log := &servertesting.FakeAuditLog{}
	s.cfg.GetAuditConfig = func() auditlog.Config {
		return auditlog.Config{
			Enabled: true,
			Target:  log,
		}
	}
	info := s.newServer(c)

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})
	conn := s.openAPIWithoutLogin(c, info)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:     user.Tag().String(),
		Credentials: password,
		CLIArgs:     "hey you guys",
	}
	err := conn.APICall("Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	// Nothing's logged at this point because there haven't been any
	// interesting requests.
	log.CheckCallNames(c)

	var addResults params.AddMachinesResults
	addReq := &params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs: []model.MachineJob{"JobHostUnits"},
		}},
	}
	err = conn.APICall("Client", 1, "", "AddMachines", addReq, &addResults)
	c.Assert(err, jc.ErrorIsNil)

	log.CheckCallNames(c, "AddConversation", "AddRequest", "AddResponse")

	convo := log.Calls()[0].Args[0].(auditlog.Conversation)
	c.Assert(convo.ConversationID, gc.HasLen, 16)
	// Blank out unknown fields.
	convo.ConversationID = "0123456789abcdef"
	convo.ConnectionID = "something"
	c.Assert(convo, gc.Equals, auditlog.Conversation{
		Who:            user.Tag().Id(),
		What:           "hey you guys",
		When:           s.Clock.Now().Format(time.RFC3339),
		ModelName:      s.Model.Name(),
		ModelUUID:      s.Model.UUID(),
		ConnectionID:   "something",
		ConversationID: "0123456789abcdef",
	})

	auditReq := log.Calls()[1].Args[0].(auditlog.Request)
	auditReq.ConversationID = ""
	auditReq.ConnectionID = ""
	auditReq.RequestID = 0
	c.Assert(auditReq, gc.Equals, auditlog.Request{
		When:    s.Clock.Now().Format(time.RFC3339),
		Facade:  "Client",
		Method:  "AddMachines",
		Version: 1,
	})
}

func (s *loginSuite) TestAuditLoggingFailureOnInterestingRequest(c *gc.C) {
	log := &servertesting.FakeAuditLog{}
	log.SetErrors(errors.Errorf("bad news bears"))
	s.cfg.GetAuditConfig = func() auditlog.Config {
		return auditlog.Config{
			Enabled: true,
			Target:  log,
		}
	}
	info := s.newServer(c)

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})
	conn := s.openAPIWithoutLogin(c, info)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:     user.Tag().String(),
		Credentials: password,
		CLIArgs:     "hey you guys",
	}
	err := conn.APICall("Admin", 3, "", "Login", request, &result)
	// No error yet since logging the conversation is deferred until
	// something happens.
	c.Assert(err, jc.ErrorIsNil)

	var addResults params.AddMachinesResults
	addReq := &params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs: []model.MachineJob{"JobHostUnits"},
		}},
	}
	err = conn.APICall("Client", 1, "", "AddMachines", addReq, &addResults)
	c.Assert(err, gc.ErrorMatches, "bad news bears")
}

func (s *loginSuite) TestAuditLoggingUsesExcludeMethods(c *gc.C) {
	log := &servertesting.FakeAuditLog{}
	s.cfg.GetAuditConfig = func() auditlog.Config {
		return auditlog.Config{
			Enabled:        true,
			ExcludeMethods: set.NewStrings("Client.AddMachines"),
			Target:         log,
		}
	}
	info := s.newServer(c)

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})
	conn := s.openAPIWithoutLogin(c, info)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:     user.Tag().String(),
		Credentials: password,
		CLIArgs:     "hey you guys",
	}
	err := conn.APICall("Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	// Nothing's logged at this point because there haven't been any
	// interesting requests.
	log.CheckCallNames(c)

	var addResults params.AddMachinesResults
	addReq := &params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs: []model.MachineJob{"JobHostUnits"},
		}},
	}
	err = conn.APICall("Client", 1, "", "AddMachines", addReq, &addResults)
	c.Assert(err, jc.ErrorIsNil)

	// Still nothing logged - the AddMachines call has been filtered out.
	log.CheckCallNames(c)

	// Call something else.
	destroyReq := &params.DestroyMachines{
		MachineNames: []string{addResults.Machines[0].Machine},
	}
	err = conn.APICall("Client", 1, "", "DestroyMachines", destroyReq, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Now the conversation and both requests are logged.
	log.CheckCallNames(c, "AddConversation", "AddRequest", "AddResponse", "AddRequest", "AddResponse")

	req1 := log.Calls()[1].Args[0].(auditlog.Request)
	c.Assert(req1.Facade, gc.Equals, "Client")
	c.Assert(req1.Method, gc.Equals, "AddMachines")

	req2 := log.Calls()[3].Args[0].(auditlog.Request)
	c.Assert(req2.Facade, gc.Equals, "Client")
	c.Assert(req2.Method, gc.Equals, "DestroyMachines")
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
}

func (s *macaroonLoginSuite) TestPublicKeyLocatorErrorIsNotPersistent(c *gc.C) {
	const remoteUser = "test@somewhere"
	s.AddModelUser(c, remoteUser)
	s.AddControllerUser(c, remoteUser, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return "test@somewhere"
	}
	srv := testserver.NewServer(c, s.StatePool, s.Controller)
	defer assertStop(c, srv)
	workingTransport := http.DefaultTransport
	failingTransport := errorTransport{
		fallback: workingTransport,
		location: s.DischargerLocation(),
		err:      errors.New("some error"),
	}
	s.PatchValue(&http.DefaultTransport, failingTransport)
	_, err := s.login(c, srv.Info)
	c.Assert(err, gc.ErrorMatches, `.*: some error .*`)

	http.DefaultTransport = workingTransport

	// The error doesn't stick around.
	_, err = s.login(c, srv.Info)
	c.Assert(err, jc.ErrorIsNil)

	// Once we've succeeded, we shouldn't try again.
	http.DefaultTransport = failingTransport

	_, err = s.login(c, srv.Info)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestLoginToController(c *gc.C) {
	// Note that currently we cannot use macaroon auth
	// to log into the controller rather than a model
	// because there's no place to store the fact that
	// a given external user is allowed access to the controller.
	s.DischargerLogin = func() string {
		return "test@somewhere"
	}
	info := s.APIInfo(c)

	// Zero the model tag so that we log into the controller
	// not the model.
	info.ModelTag = names.ModelTag{}

	client, err := api.Open(info, api.DialOpts{})
	assertInvalidEntityPassword(c, err)
	c.Assert(client, gc.Equals, nil)
}

func (s *macaroonLoginSuite) login(c *gc.C, info *api.Info) (params.LoginResult, error) {
	cookieJar := apitesting.NewClearableCookieJar()

	infoSkipLogin := *info
	infoSkipLogin.SkipLogin = true
	infoSkipLogin.Macaroons = nil
	client := s.OpenAPI(c, &infoSkipLogin, cookieJar)
	defer client.Close()

	var (
		request params.LoginRequest
		result  params.LoginResult
	)
	err := client.APICall("Admin", 3, "", "Login", &request, &result)
	if err != nil {
		return params.LoginResult{}, errors.Annotatef(err, "cannot log in")
	}

	cookieURL := &url.URL{
		Scheme: "https",
		Host:   "localhost",
		Path:   "/",
	}

	bakeryClient := httpbakery.NewClient()

	mac := result.BakeryDischargeRequired
	if mac == nil {
		var err error
		mac, err = bakery.NewLegacyMacaroon(result.DischargeRequired)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = bakeryClient.HandleError(context.Background(), cookieURL, &httpbakery.Error{
		Message: result.DischargeRequiredReason,
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     mac,
			MacaroonPath: "/",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Add the macaroons that have been saved by HandleError to our login request.
	request.Macaroons = httpbakery.MacaroonsForURL(bakeryClient.Client.Jar, cookieURL)

	err = client.APICall("Admin", 3, "", "Login", &request, &result)
	return result, err
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerNoAccess(c *gc.C) {
	s.DischargerLogin = func() string {
		return "test@somewhere"
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	_, err := s.login(c, info)
	assertInvalidEntityPassword(c, err)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerLoginAccess(c *gc.C) {
	setEveryoneAccess(c, s.State, s.AdminUserTag(c), permission.LoginAccess)
	const remoteUser = "test@somewhere"
	var remoteUserTag = names.NewUserTag(remoteUser)

	s.DischargerLogin = func() string {
		return remoteUser
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	result, err := s.login(c, info)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	c.Check(result.UserInfo.Identity, gc.Equals, remoteUserTag.String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "")
	c.Check(result.Servers, gc.DeepEquals, params.FromProviderHostsPorts(parseHostPortsFromAddress(c, info.Addrs...)))
}

func parseHostPortsFromAddress(c *gc.C, addresses ...string) []network.ProviderHostPorts {
	hps := make([]network.ProviderHostPorts, len(addresses))
	for i, add := range addresses {
		hp, err := network.ParseProviderHostPorts(add)
		c.Assert(err, jc.ErrorIsNil)
		hps[i] = hp
	}
	return hps
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerSuperuserAccess(c *gc.C) {
	setEveryoneAccess(c, s.State, s.AdminUserTag(c), permission.SuperuserAccess)
	const remoteUser = "test@somewhere"
	var remoteUserTag = names.NewUserTag(remoteUser)

	s.DischargerLogin = func() string {
		return remoteUser
	}
	info := s.APIInfo(c)
	// Log in to the controller, not the model.
	info.ModelTag = names.ModelTag{}

	result, err := s.login(c, info)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	c.Check(result.UserInfo.Identity, gc.Equals, remoteUserTag.String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "superuser")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "")
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelNoExplicitAccess(c *gc.C) {
	// If we have a remote user which the controller knows nothing about,
	// and the macaroon is discharged successfully, and the user is attempting
	// to log into a model, that is permission denied.
	setEveryoneAccess(c, s.State, s.AdminUserTag(c), permission.LoginAccess)
	s.DischargerLogin = func() string {
		return "test@somewhere"
	}
	info := s.APIInfo(c)

	_, err := s.login(c, info)
	assertPermissionDenied(c, err)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithExplicitAccess(c *gc.C) {
	s.testRemoteUserLoginToModelWithExplicitAccess(c, false)
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithExplicitAccessAndAllowModelAccess(c *gc.C) {
	s.testRemoteUserLoginToModelWithExplicitAccess(c, true)
}

func (s *macaroonLoginSuite) testRemoteUserLoginToModelWithExplicitAccess(c *gc.C, allowModelAccess bool) {
	cfg := testserver.DefaultServerConfig(c, nil)
	cfg.AllowModelAccess = allowModelAccess
	cfg.Controller = s.Controller
	srv := testserver.NewServerWithConfig(c, s.StatePool, cfg)
	defer assertStop(c, srv)
	srv.Info.ModelTag = s.Model.ModelTag()

	// If we have a remote user which has explicit model access, but neither
	// controller access nor 'everyone' access, the user will have access
	// only if the AllowModelAccess configuration flag is true.
	const remoteUser = "test@somewhere"
	s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User: remoteUser,

		Access: permission.WriteAccess,
	})
	s.DischargerLogin = func() string {
		return remoteUser
	}

	_, err := s.login(c, srv.Info)
	if allowModelAccess {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		assertPermissionDenied(c, err)
	}
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToModelWithControllerAccess(c *gc.C) {
	const remoteUser = "test@somewhere"
	var remoteUserTag = names.NewUserTag(remoteUser)
	s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User:   remoteUser,
		Access: permission.WriteAccess,
	})
	s.AddControllerUser(c, remoteUser, permission.SuperuserAccess)

	s.DischargerLogin = func() string {
		return remoteUser
	}
	info := s.APIInfo(c)

	result, err := s.login(c, info)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	c.Check(result.UserInfo.Identity, gc.Equals, remoteUserTag.String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "superuser")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "write")
}

func (s *macaroonLoginSuite) TestLoginToModelSuccess(c *gc.C) {
	const remoteUser = "test@somewhere"
	s.AddModelUser(c, remoteUser)
	s.AddControllerUser(c, remoteUser, permission.LoginAccess)
	s.DischargerLogin = func() string {
		return "test@somewhere"
	}
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	client, err := api.Open(s.APIInfo(c), api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer client.Close()

	// The auth tag has been correctly returned by the server.
	c.Assert(client.AuthTag(), gc.Equals, names.NewUserTag(remoteUser))
}

func (s *macaroonLoginSuite) TestFailedToObtainDischargeLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return ""
	}
	client, err := api.Open(s.APIInfo(c), api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(client, gc.Equals, nil)
}

func (s *macaroonLoginSuite) TestUnknownUserLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return "testUnknown@somewhere"
	}
	client, err := api.Open(s.APIInfo(c), api.DialOpts{})
	assertInvalidEntityPassword(c, err)
	c.Assert(client, gc.Equals, nil)
}

func assertInvalidEntityPassword(c *gc.C, err error) {
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})
}

func assertPermissionDenied(c *gc.C, err error) {
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "permission denied",
		Code:    "unauthorized access",
	})
}

func setEveryoneAccess(c *gc.C, st *state.State, adminUser names.UserTag, access permission.Access) {
	err := controller.ChangeControllerAccess(
		st, adminUser, names.NewUserTag(common.EveryoneTagName),
		params.GrantControllerAccess, access)
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&migrationSuite{})

type migrationSuite struct {
	baseLoginSuite
}

func (s *migrationSuite) TestImportingModel(c *gc.C) {
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "nonce",
	})
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	// Users should be able to log in but RPC requests should fail.
	info := s.APIInfo(c)
	userConn := s.OpenAPIAs(c, info.Tag, info.Password)
	defer userConn.Close()
	_, err = userConn.Client().Status(nil)
	c.Check(err, gc.ErrorMatches, "migration in progress, model is importing")

	// Machines should be able to use the API.
	machineConn := s.OpenAPIAsMachine(c, m.Tag(), password, "nonce")
	defer machineConn.Close()
	_, err = apimachiner.NewState(machineConn).Machine(m.MachineTag())
	c.Check(err, jc.ErrorIsNil)
}

func (s *migrationSuite) TestExportingModel(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	// Users should be able to log in but RPC requests should fail.
	info := s.APIInfo(c)
	userConn := s.OpenAPIAs(c, info.Tag, info.Password)
	defer userConn.Close()

	// Status is fine.
	_, err = userConn.Client().Status(nil)
	c.Check(err, jc.ErrorIsNil)

	// Modifying commands like destroy machines are not.
	err = userConn.Client().DestroyMachines("42")
	c.Check(err, gc.ErrorMatches, "model migration in progress")
}

type loginV3Suite struct {
	baseLoginSuite
}

var _ = gc.Suite(&loginV3Suite{})

func (s *loginV3Suite) TestClientLoginToModel(c *gc.C) {
	info := s.APIInfo(c)
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	client := apiState.Client()
	_, err = client.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginV3Suite) TestClientLoginToController(c *gc.C) {
	info := s.APIInfo(c)
	info.ModelTag = names.ModelTag{}
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	client := apiState.Client()
	_, err = client.GetModelConstraints()
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `facade "Client" not supported for controller API connection`,
		Code:    "not supported",
	})
}

func (s *loginV3Suite) TestClientLoginToControllerNoAccessToControllerModel(c *gc.C) {
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
	info := s.APIInfo(c)
	info.Tag = nil
	info.Password = ""
	info.ModelTag = names.ModelTag{}
	info.SkipLogin = true
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	err = apiState.APICall("Admin", 2, "", "Login", struct{}{}, nil)
	c.Assert(err, gc.ErrorMatches, ".*this version of Juju does not support login from old clients.*")
}

// errorTransport implements http.RoundTripper by always
// returning the given error from RoundTrip when it visits
// the given URL (otherwise it uses the fallback transport.
type errorTransport struct {
	err      error
	location string
	fallback http.RoundTripper
}

func (t errorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() == t.location+"/publickey" {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, t.err
	}
	if req.URL.String() == t.location+"/discharge/info" {
		if req.Body != nil {
			req.Body.Close()
		}
		return &http.Response{
			Request:    req,
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": {"application/text"}},
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(""))),
		}, nil
	}
	return t.fallback.RoundTrip(req)
}

type mockDelayAuthenticator struct {
	httpcontext.LocalMacaroonAuthenticator
	delay chan struct{}
}

func (a *mockDelayAuthenticator) AuthenticateLoginRequest(
	serverHost string,
	modelUUID string,
	req params.LoginRequest,
) (httpcontext.AuthInfo, error) {
	select {
	case <-time.After(coretesting.LongWait):
		panic("timed out delaying login")
	case <-a.delay:
	}
	tag, err := names.ParseTag(req.AuthTag)
	if err != nil {
		return httpcontext.AuthInfo{}, errors.Trace(err)
	}
	return httpcontext.AuthInfo{Entity: &mockEntity{tag: tag}}, nil
}
