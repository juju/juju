// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	apiclient "github.com/juju/juju/api/client/client"
	machineclient "github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/controller"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

const (
	clientFacadeVersion           = 6
	machineManagerFacadeVersion   = 9
	userManagerFacadeVersion      = 3
	sshClientFacadeVersion        = 4
	pingerFacadeVersion           = 1
	modelManagerFacadeVersion     = 10
	highAvailabilityFacadeVersion = 2
)

type baseLoginSuite struct {
	jujutesting.ApiServerSuite
	mgmtSpace *state.Space
}

func (s *baseLoginSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)

	var err error
	s.mgmtSpace, err = s.ControllerModel(c).State().AddSpace("mgmt01", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.ControllerModel(c).State().UpdateControllerConfig(map[string]interface{}{corecontroller.JujuManagementSpace: "mgmt01"}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

type loginSuite struct {
	jujutesting.ApiServerSuite
}

var _ = gc.Suite(&loginSuite{})

func (s *loginSuite) SetUpTest(c *gc.C) {
	s.Clock = testclock.NewDilatedWallClock(time.Second)
	s.ApiServerSuite.SetUpTest(c)
}

// openAPIWithoutLogin connects to the API and returns an api connection
// without actually calling st.Login already.
func (s *loginSuite) openAPIWithoutLogin(c *gc.C) api.Connection {
	return s.openModelAPIWithoutLogin(c, s.ControllerModelUUID())
}

func (s *loginSuite) openModelAPIWithoutLogin(c *gc.C, modelUUID string) api.Connection {
	info := s.ModelApiInfo(modelUUID)
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	conn, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

func (s *loginSuite) TestLoginWithInvalidTag(c *gc.C) {
	st := s.openAPIWithoutLogin(c)

	request := &params.LoginRequest{
		AuthTag:       "bar",
		Credentials:   "password",
		ClientVersion: jujuversion.Current.String(),
	}

	var response params.LoginResult
	err := st.APICall(context.Background(), "Admin", 3, "", "Login", request, &response)
	c.Assert(err, gc.ErrorMatches, `.*"bar" is not a valid tag.*`)
}

func (s *loginSuite) TestBadLogin(c *gc.C) {
	for i, t := range []struct {
		tag      names.Tag
		password string
		err      error
		code     string
	}{{
		tag:      jujutesting.AdminUser,
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
			st := s.openAPIWithoutLogin(c)

			_, err := apimachiner.NewClient(st).Machine(context.Background(), names.NewMachineTag("0"))
			c.Assert(err, gc.NotNil)
			c.Check(errors.Is(err, errors.NotImplemented), jc.IsTrue)
			c.Check(strings.Contains(err.Error(), `unknown facade type "Machiner"`), jc.IsTrue)

			// Since these are user login tests, the nonce is empty.
			err = st.Login(context.Background(), t.tag, t.password, "", nil)
			c.Assert(errors.Cause(err), gc.DeepEquals, t.err)
			c.Assert(params.ErrCode(err), gc.Equals, t.code)

			_, err = apimachiner.NewClient(st).Machine(context.Background(), names.NewMachineTag("0"))
			c.Assert(err, gc.NotNil)
			c.Check(errors.Is(err, errors.NotImplemented), jc.IsTrue)
			c.Check(strings.Contains(err.Error(), `unknown facade type "Machiner"`), jc.IsTrue)
		}()
	}
}

func (s *loginSuite) TestLoginAsDeactivatedUser(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	st := s.openAPIWithoutLogin(c)
	password := "password"
	u := f.MakeUser(c, &factory.UserParams{Password: password, Disabled: true})

	_, err := apiclient.NewClient(st, coretesting.NoopLogger{}).Status(nil)
	c.Assert(err, gc.NotNil)
	c.Check(errors.Is(err, errors.NotImplemented), jc.IsTrue)
	c.Check(strings.Contains(err.Error(), `unknown facade type "Client"`), jc.IsTrue)

	// Since these are user login tests, the nonce is empty.
	err = st.Login(context.Background(), u.Tag(), password, "", nil)

	// The error message should not leak that the user is disabled.
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})

	_, err = apiclient.NewClient(st, coretesting.NoopLogger{}).Status(nil)
	c.Assert(err, gc.NotNil)
	c.Check(errors.Is(err, errors.NotImplemented), jc.IsTrue)
	c.Check(strings.Contains(err.Error(), `unknown facade type "Client"`), jc.IsTrue)
}

func (s *loginSuite) TestLoginAsDeletedUser(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	st := s.openAPIWithoutLogin(c)
	password := "password"
	u := f.MakeUser(c, &factory.UserParams{Password: password})

	_, err := apiclient.NewClient(st, coretesting.NoopLogger{}).Status(nil)
	c.Assert(err, gc.NotNil)
	c.Check(errors.Is(err, errors.NotImplemented), jc.IsTrue)
	c.Check(strings.Contains(err.Error(), `unknown facade type "Client"`), jc.IsTrue)

	err = s.ControllerModel(c).State().RemoveUser(u.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	// Since these are user login tests, the nonce is empty.
	err = st.Login(context.Background(), u.Tag(), password, "", nil)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})

	_, err = apiclient.NewClient(st, coretesting.NoopLogger{}).Status(nil)
	c.Assert(err, gc.NotNil)
	c.Check(errors.Is(err, errors.NotImplemented), jc.IsTrue)
	c.Check(strings.Contains(err.Error(), `unknown facade type "Client"`), jc.IsTrue)
}

func (s *loginSuite) setupManagementSpace(c *gc.C) *state.Space {
	mgmtSpace, err := s.ControllerModel(c).State().AddSpace("mgmt01", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	cfg := map[string]interface{}{
		corecontroller.JujuManagementSpace: "mgmt01",
	}

	err = s.ControllerModel(c).State().UpdateControllerConfig(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.ControllerServiceFactory(c).ControllerConfig().UpdateControllerConfig(context.Background(), cfg, nil)
	c.Assert(err, jc.ErrorIsNil)

	return mgmtSpace
}

func (s *loginSuite) addController(c *gc.C) (state.ControllerNode, string) {
	node, err := s.ControllerModel(c).State().AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	password, err := password.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = node.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	return node, password
}

func (s *loginSuite) TestControllerAgentLogin(c *gc.C) {
	// The agent login tests also check the management space.
	mgmtSpace := s.setupManagementSpace(c)
	info := s.ControllerModelApiInfo()

	node, password := s.addController(c)
	info.Tag = node.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"

	s.assertAgentLogin(c, info, mgmtSpace)
}

func (s *loginSuite) TestLoginAddressesForAgents(c *gc.C) {
	// The agent login tests also check the management space.
	mgmtSpace := s.setupManagementSpace(c)

	info := s.ControllerModelApiInfo()
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
	st := s.ControllerModel(c).State()

	cfg, err := s.ControllerServiceFactory(c).ControllerConfig().ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetAPIHostPorts(cfg, nil, nil)
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
		network.NewSpaceAddress("server-1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
	}
	server1Addresses[1].SpaceID = mgmtSpace.Id()

	server2Addresses := network.SpaceAddresses{
		network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)),
	}

	err = st.SetAPIHostPorts(cfg, []network.SpaceHostPorts{
		network.SpaceAddressesWithPort(server1Addresses, 123),
		network.SpaceAddressesWithPort(server2Addresses, 456),
	}, []network.SpaceHostPorts{
		network.SpaceAddressesWithPort(network.SpaceAddresses{server1Addresses[1]}, 123),
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

	info := s.ControllerModelApiInfo()
	info = s.infoForNewUser(c, info)

	server1Addresses := network.SpaceAddresses{
		network.NewSpaceAddress("server-1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
	}
	server1Addresses[1].SpaceID = mgmtSpace.Id()

	server2Addresses := network.SpaceAddresses{
		network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)),
	}

	cfg := coretesting.FakeControllerConfig()
	st := s.ControllerModel(c).State()

	newAPIHostPorts := []network.SpaceHostPorts{
		network.SpaceAddressesWithPort(server1Addresses, 123),
		network.SpaceAddressesWithPort(server2Addresses, 456),
	}
	err := st.SetAPIHostPorts(cfg, newAPIHostPorts, newAPIHostPorts)
	c.Assert(err, jc.ErrorIsNil)

	exp := []network.MachineHostPorts{
		{
			{
				MachineAddress: network.NewMachineAddress("server-1", network.WithScope(network.ScopePublic)),
				NetPort:        123,
			},
			{
				MachineAddress: network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
				NetPort:        123,
			},
		}, {
			{
				MachineAddress: network.NewMachineAddress("::1", network.WithScope(network.ScopeMachineLocal)),
				NetPort:        456,
			},
		},
	}

	_, hostPorts := s.loginHostPorts(c, info)
	// Ignoring the address used to login, the returned API addresses should not
	// Have management space filtering applied.
	c.Assert(hostPorts[1:], gc.DeepEquals, exp)
}

func (s *loginSuite) infoForNewMachine(c *gc.C, info *api.Info) *api.Info {
	// Make a copy
	newInfo := *info

	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()
	machine, password := f.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	newInfo.Tag = machine.Tag()
	newInfo.Password = password
	newInfo.Nonce = "fake_nonce"
	return &newInfo
}

func (s *loginSuite) infoForNewUser(c *gc.C, info *api.Info) *api.Info {
	// Make a copy
	newInfo := *info

	userTag := names.NewUserTag("charlie")
	password := "shhh..."

	userService := s.ControllerServiceFactory(c).Access()
	_, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        userTag.Name(),
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(password)),
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
	})
	c.Assert(err, jc.ErrorIsNil)

	// TODO (stickupkid): Remove the make user call when permissions are
	// written to state.
	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()

	f.MakeUser(c, &factory.UserParams{
		Name:        userTag.Name(),
		DisplayName: "Charlie Brown",
		Password:    password,
	})

	newInfo.Tag = userTag
	newInfo.Password = password
	return &newInfo
}

func (s *loginSuite) TestNonModelUserLoginFails(c *gc.C) {
	info := s.ControllerModelApiInfo()
	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()
	user := f.MakeUser(c, &factory.UserParams{Password: "dummy-password", NoModelUser: true})
	ctag := names.NewControllerTag(s.ControllerModel(c).State().ControllerUUID())
	err := s.ControllerModel(c).State().RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	info.Password = "dummy-password"
	info.Tag = user.UserTag()
	_, err = api.Open(info, fastDialOpts)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) TestLoginValidationDuringUpgrade(c *gc.C) {
	s.WithUpgrading = true
	s.testLoginDuringMaintenance(c, func(st api.Connection) {
		var statusResult params.FullStatus
		err := st.APICall(context.Background(), "Client", clientFacadeVersion, "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, jc.ErrorIsNil)

		err = st.APICall(context.Background(), "Client", clientFacadeVersion, "", "ModelSet", params.ModelSet{}, nil)
		c.Assert(err, jc.Satisfies, params.IsCodeUpgradeInProgress)
	})
}

func (s *loginSuite) testLoginDuringMaintenance(c *gc.C, check func(api.Connection)) {
	st := s.openAPIWithoutLogin(c)
	err := st.Login(context.Background(), jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	check(st)
}

func (s *loginSuite) TestMachineLoginDuringMaintenance(c *gc.C) {
	s.WithUpgrading = true
	info := s.ControllerModelApiInfo()
	machine := s.infoForNewMachine(c, info)
	_, err := api.Open(machine, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, `login for machine \d+ blocked because upgrade is in progress`)
}

func (s *loginSuite) TestControllerMachineLoginDuringMaintenance(c *gc.C) {
	s.WithUpgrading = true
	info := s.ControllerModelApiInfo()

	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()
	machine, password := f.MakeMachineReturningPassword(c, &factory.MachineParams{
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
	s.WithUpgrading = true
	info := s.ControllerModelApiInfo()

	node, password := s.addController(c)
	info.Tag = node.Tag()
	info.Password = password

	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.Close(), jc.ErrorIsNil)
}

func (s *loginSuite) TestMigratedModelLogin(c *gc.C) {
	info := s.ControllerModelApiInfo()
	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()
	modelOwner := f.MakeUser(c, &factory.UserParams{
		Password: "secret",
	})
	modelState := f.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
	})
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	controllerTag := names.NewControllerTag(uuid.MustNewUUID().String())

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
	}, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig.SetPhase(phase, status.NoopStatusHistoryRecorder), jc.ErrorIsNil)
	}
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(modelState.RemoveDyingModel(), jc.ErrorIsNil)

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
	_, ok = errors.Cause(err).(*api.RedirectError)
	c.Assert(ok, gc.Equals, true)
}

func (s *loginSuite) TestAnonymousModelLogin(c *gc.C) {
	conn := s.openAPIWithoutLogin(c)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag: names.NewUserTag(api.AnonymousUsername).String(),
	}
	err := conn.APICall(context.Background(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.IsNil)
	c.Assert(result.ControllerTag, gc.Equals, s.ControllerModel(c).State().ControllerTag().String())
	c.Assert(result.ModelTag, gc.Equals, names.NewModelTag(s.ControllerModelUUID()).String())
	c.Assert(result.Facades, jc.DeepEquals, []params.FacadeVersions{
		{Name: "CrossModelRelations", Versions: []int{3}},
		{Name: "CrossModelSecrets", Versions: []int{1}},
		{Name: "NotifyWatcher", Versions: []int{1}},
		{Name: "OfferStatusWatcher", Versions: []int{1}},
		{Name: "RelationStatusWatcher", Versions: []int{1}},
		{Name: "RelationUnitsWatcher", Versions: []int{1}},
		{Name: "RemoteRelationWatcher", Versions: []int{1}},
		{Name: "SecretsRevisionWatcher", Versions: []int{1}},
		{Name: "StringsWatcher", Versions: []int{1}},
	})
}

func (s *loginSuite) TestAnonymousControllerLogin(c *gc.C) {
	conn := s.openModelAPIWithoutLogin(c, "")

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       names.NewUserTag(api.AnonymousUsername).String(),
		ClientVersion: jujuversion.Current.String(),
	}
	err := conn.APICall(context.Background(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.IsNil)
	c.Assert(result.ControllerTag, gc.Equals, s.ControllerModel(c).State().ControllerTag().String())
	c.Assert(result.Facades, jc.DeepEquals, []params.FacadeVersions{
		{Name: "CrossController", Versions: []int{1}},
		{Name: "NotifyWatcher", Versions: []int{1}},
	})
}

func (s *loginSuite) TestControllerModel(c *gc.C) {
	st := s.openAPIWithoutLogin(c)

	err := st.Login(context.Background(), jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertRemoteModel(c, st, s.ControllerModel(c).ModelTag())
}

func (s *loginSuite) TestControllerModelBadCreds(c *gc.C) {
	st := s.openAPIWithoutLogin(c)

	err := st.Login(context.Background(), jujutesting.AdminUser, "bad-password", "", nil)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) TestNonExistentModel(c *gc.C) {
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	st := s.openModelAPIWithoutLogin(c, uuid.String())

	err = st.Login(context.Background(), jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: fmt.Sprintf("unknown model: %q", uuid),
		Code:    "model not found",
	})
}

func (s *loginSuite) TestInvalidModel(c *gc.C) {
	info := s.ControllerModelApiInfo()
	info.ModelTag = names.NewModelTag("rubbish")
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, `unable to connect to API: invalid model UUID "rubbish" \(Bad Request\)`)
	c.Assert(st, gc.IsNil)
}

func (s *loginSuite) TestOtherModel(c *gc.C) {
	userTag := names.NewUserTag("charlie")
	password := "shhh..."

	userService := s.ControllerServiceFactory(c).Access()
	_, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        userTag.Name(),
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(password)),
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
	})
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeUser(c, &factory.UserParams{
		Name: userTag.Name(),
	})
	modelState := f.MakeModel(c, &factory.ModelParams{
		Owner: userTag,
	})
	defer modelState.Close()

	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	st := s.openModelAPIWithoutLogin(c, model.UUID())

	err = st.Login(context.Background(), userTag, password, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertRemoteModel(c, st, model.ModelTag())
}

func (s *loginSuite) TestMachineLoginOtherModel(c *gc.C) {
	// User credentials are checked against a global user list.
	// Machine credentials are checked against model specific
	// machines, so this makes sure that the credential checking is
	// using the correct state connection.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	modelOwner := f.MakeUser(c, nil)
	modelState := f.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
		ConfigAttrs: map[string]interface{}{
			"controller": false,
		},
	})
	defer modelState.Close()

	f2, release := s.NewFactory(c, modelState.ModelUUID())
	defer release()
	machine, password := f2.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "test-nonce",
	})

	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	st := s.openModelAPIWithoutLogin(c, model.UUID())

	err = st.Login(context.Background(), machine.Tag(), password, "test-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestMachineLoginOtherModelNotProvisioned(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	modelOwner := f.MakeUser(c, nil)
	modelState := f.MakeModel(c, &factory.ModelParams{
		Owner: modelOwner.UserTag(),
		ConfigAttrs: map[string]interface{}{
			"controller": false,
		},
	})
	defer modelState.Close()

	f2, release := s.NewFactory(c, modelState.ModelUUID())
	defer release()
	machine, password := f2.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})

	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	st := s.openModelAPIWithoutLogin(c, model.UUID())

	// If the agent attempts Login before the provisioner has recorded
	// the machine's nonce in state, then the agent should get back an
	// error with code "not provisioned".
	err = st.Login(context.Background(), machine.Tag(), password, "nonce", nil)
	c.Assert(err, gc.ErrorMatches, `machine 0 not provisioned \(not provisioned\)`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
}

func (s *loginSuite) TestOtherModelFromController(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	machine, password := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})

	defer release()
	modelState := f.MakeModel(c, nil)
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	info := s.ModelApiInfo(model.UUID())
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	conn, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	err = conn.Login(context.Background(), machine.Tag(), password, "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherModelFromControllerOtherNotProvisioned(c *gc.C) {
	info := s.ControllerModelApiInfo()

	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()
	managerMachine, password := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})

	// Create a hosted model with an unprovisioned machine that has the
	// same tag as the manager machine.
	hostedModelState := f.MakeModel(c, nil)
	defer hostedModelState.Close()
	f2, release := s.NewFactory(c, hostedModelState.ModelUUID())
	defer release()
	workloadMachine, _ := f2.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})
	c.Assert(managerMachine.Tag(), gc.Equals, workloadMachine.Tag())

	hostedModel, err := hostedModelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	info.ModelTag = hostedModel.ModelTag()
	st := s.openAPIWithoutLogin(c)

	// The fact that the machine with the same tag in the hosted
	// model is unprovisioned should not cause the login to fail
	// with "not provisioned", because the passwords don't match.
	err = st.Login(context.Background(), managerMachine.Tag(), password, "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherModelWhenNotController(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	machine, password := f.MakeMachineReturningPassword(c, nil)

	modelState := f.MakeModel(c, nil)
	defer modelState.Close()

	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	st := s.openModelAPIWithoutLogin(c, model.UUID())

	err = st.Login(context.Background(), machine.Tag(), password, "nonce", nil)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) loginLocalUser(c *gc.C, info *api.Info) (names.UserTag, params.LoginResult) {
	userTag := names.NewUserTag("charlie")
	password := "shhh..."

	userService := s.ControllerServiceFactory(c).Access()
	_, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        userTag.Name(),
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(password)),
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
	})
	c.Assert(err, jc.ErrorIsNil)

	// TODO (stickupkid): Remove the make user call when permissions are
	// written to state.
	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()

	f.MakeUser(c, &factory.UserParams{
		Name:     userTag.Name(),
		Password: password,
	})

	conn := s.openAPIWithoutLogin(c)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       userTag.String(),
		Credentials:   password,
		ClientVersion: jujuversion.Current.String(),
	}
	err = conn.APICall(context.Background(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	return userTag, result
}

func (s *loginSuite) TestLoginResultLocalUser(c *gc.C) {
	info := s.ControllerModelApiInfo()

	userTag, result := s.loginLocalUser(c, info)
	c.Check(result.UserInfo.Identity, gc.Equals, userTag.String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "admin")
}

func (s *loginSuite) TestLoginResultLocalUserEveryoneCreateOnlyNonLocal(c *gc.C) {
	info := s.ControllerModelApiInfo()

	setEveryoneAccess(c, s.ControllerModel(c).State(), jujutesting.AdminUser, permission.SuperuserAccess)

	userTag, result := s.loginLocalUser(c, info)
	c.Check(result.UserInfo.Identity, gc.Equals, userTag.String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "admin")
}

func (s *loginSuite) assertRemoteModel(c *gc.C, conn api.Connection, expected names.ModelTag) {
	// Look at what the api thinks it has.
	tag, ok := conn.ModelTag()
	c.Assert(ok, jc.IsTrue)
	c.Assert(tag, gc.Equals, expected)
	// Look at what the api Client thinks it has.
	client := modelconfig.NewClient(conn)

	// The code below is to verify that the API connection is operating on
	// the expected model. We make a change in state on that model, and
	// then check that it is picked up by a call to the API.

	m, release := s.ApiServerSuite.Model(c, tag.Id())
	defer release()

	expectedCons := constraints.MustParse("mem=8G")
	err := m.State().SetModelConstraints(expectedCons)
	c.Assert(err, jc.ErrorIsNil)

	cons, err := client.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, expectedCons)
}

func (s *loginSuite) TestLoginUpdatesLastLoginAndConnection(c *gc.C) {
	userService := s.ControllerServiceFactory(c).Access()
	uuid, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("password")),
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
	})
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	now := s.Clock.Now().UTC()

	info := s.ControllerModelApiInfo()
	info.Tag = names.NewUserTag("bobbrown")
	info.Password = "password"

	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	// The user now has last login updated.
	user, err := userService.GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(user.LastLogin, jc.Almost, now)

	// The model user is also updated.
	model := s.ControllerModel(c)
	modelUser, err := model.State().UserAccess(names.NewUserTag("bobbrown"), model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when, jc.Almost, now)
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	m, password := f.MakeMachineReturningPassword(c, &factory.MachineParams{Nonce: "nonce"})

	model, err := s.ControllerModel(c).State().Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	// Users should be able to log in but RPC requests should fail.
	userConn := s.OpenControllerModelAPI(c)
	defer userConn.Close()
	_, err = apiclient.NewClient(userConn, coretesting.NoopLogger{}).Status(nil)
	c.Check(err, gc.ErrorMatches, "migration in progress, model is importing")

	// Machines should be able to use the API.
	machineConn := s.OpenModelAPIAs(c, s.ControllerModelUUID(), m.Tag(), password, "nonce")
	_, err = apimachiner.NewClient(machineConn).Machine(context.Background(), m.MachineTag())
	c.Check(err, jc.ErrorIsNil)
}

func (s *migrationSuite) TestExportingModel(c *gc.C) {
	model, err := s.ControllerModel(c).State().Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)

	// Users should be able to log in but RPC requests should fail.
	userConn := s.OpenControllerModelAPI(c)
	defer userConn.Close()

	// Status is fine.
	_, err = apiclient.NewClient(userConn, coretesting.NoopLogger{}).Status(nil)
	c.Check(err, jc.ErrorIsNil)

	// Modifying commands like destroy machines are not.
	_, err = machineclient.NewClient(userConn).DestroyMachinesWithParams(false, false, false, nil, "42")
	c.Check(err, gc.ErrorMatches, "model migration in progress")
}

type loginV3Suite struct {
	baseLoginSuite
}

var _ = gc.Suite(&loginV3Suite{})

func (s *loginV3Suite) TestClientLoginToModel(c *gc.C) {
	apiState := s.OpenControllerModelAPI(c)
	client := modelconfig.NewClient(apiState)
	_, err := client.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginV3Suite) TestClientLoginToController(c *gc.C) {
	apiState := s.OpenControllerAPI(c)
	client := machineclient.NewClient(apiState)
	_, err := client.RetryProvisioning(false, names.NewMachineTag("machine-0"))
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `facade "MachineManager" not supported for controller API connection`,
		Code:    "not supported",
	})
}

func (s *loginV3Suite) TestClientLoginToControllerNoAccessToControllerModel(c *gc.C) {
	userService := s.ControllerServiceFactory(c).Access()
	uuid, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("password")),
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
	})
	c.Assert(err, jc.ErrorIsNil)

	// TODO (stickupkid): Permissions: This is only required to insert admin
	// permissions into the state, remove when permissions are written to state.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeUser(c, &factory.UserParams{
		Name: "bobbrown",
	})

	now := s.Clock.Now().UTC().Truncate(time.Second)

	s.OpenControllerAPIAs(c, names.NewUserTag("bobbrown"), "password")

	user, err := userService.GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(user.LastLogin, gc.Not(jc.Before), now)
}

func (s *loginV3Suite) TestClientLoginToRootOldClient(c *gc.C) {
	info := s.ControllerModelApiInfo()
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	apiState, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	err = apiState.APICall(context.Background(), "Admin", 2, "", "Login", struct{}{}, nil)
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
			Body:       io.NopCloser(bytes.NewReader([]byte(""))),
		}, nil
	}
	return t.fallback.RoundTrip(req)
}

func ptr[T any](v T) *T {
	return &v
}
