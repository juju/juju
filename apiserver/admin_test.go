// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	apiclient "github.com/juju/juju/api/client/client"
	machineclient "github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	accessservice "github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/domain/controllernode"
	"github.com/juju/juju/internal/auth"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

const (
	clientFacadeVersion           = 8
	machineManagerFacadeVersion   = 11
	sshClientFacadeVersion        = 4
	pingerFacadeVersion           = 1
	modelManagerFacadeVersion     = 10
	highAvailabilityFacadeVersion = 2
)

type baseLoginSuite struct {
	jujutesting.ApiServerSuite
}

func TestLoginStub(t *stdtesting.T) {
	t.Skipf(`This suite is missing tests for the following scenarios:
 - Machine login during maintenance
 - Controller agent login
 - Controller agent login during maintenance
 - Machine login other model
 - Test login from another model whilst controller
 - Test login from another model whilst controller, but machine not provisioned
 - Test login from another model not controller should error out
 - Test login during model migration
 - Test login for agents
 - Test login for agents with machine not provisioned
 - Test login addresses as user
 - Test login addresses as not user
	`)
}

func (s *baseLoginSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)

	controllerNodeService := s.ControllerDomainServices(c).ControllerNode()
	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.9.9.32",
				},
			},
			NetPort: 42,
		},
	}
	err := controllerNodeService.SetAPIAddresses(c.Context(), controllernode.SetAPIAddressArgs{
		APIAddresses: map[string]network.SpaceHostPorts{
			"0": addrs,
		},
	})
	c.Assert(err, tc.IsNil)
}

type loginSuite struct {
	jujutesting.ApiServerSuite
}

func TestLoginSuite(t *stdtesting.T) {
	tc.Run(t, &loginSuite{})
}

func (s *loginSuite) SetUpTest(c *tc.C) {
	s.Clock = testclock.NewDilatedWallClock(time.Second)
	s.ApiServerSuite.SetUpTest(c)

	controllerNodeService := s.ControllerDomainServices(c).ControllerNode()
	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.9.9.32",
				},
			},
			NetPort: 42,
		},
	}
	err := controllerNodeService.SetAPIAddresses(c.Context(), controllernode.SetAPIAddressArgs{
		APIAddresses: map[string]network.SpaceHostPorts{
			"0": addrs,
		},
	})
	c.Assert(err, tc.IsNil)
}

// openAPIWithoutLogin connects to the API and returns an api connection
// without actually calling st.Login already.
func (s *loginSuite) openAPIWithoutLogin(c *tc.C) api.Connection {
	return s.openModelAPIWithoutLogin(c, s.ControllerModelUUID())
}

func (s *loginSuite) openModelAPIWithoutLogin(c *tc.C, modelUUID string) api.Connection {
	info := s.ModelApiInfo(modelUUID)
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	conn, err := api.Open(c.Context(), info, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	return conn
}

func (s *loginSuite) TestLoginWithInvalidTag(c *tc.C) {
	st := s.openAPIWithoutLogin(c)

	request := &params.LoginRequest{
		AuthTag:       "bar",
		Credentials:   "password",
		ClientVersion: jujuversion.Current.String(),
	}

	var response params.LoginResult
	err := st.APICall(c.Context(), "Admin", 3, "", "Login", request, &response)
	c.Assert(err, tc.ErrorMatches, `.*"bar" is not a valid tag.*`)
}

func (s *loginSuite) TestBadLogin(c *tc.C) {
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

			_, err := apimachiner.NewClient(st).Machine(c.Context(), names.NewMachineTag("0"))
			c.Assert(err, tc.NotNil)
			c.Check(errors.Is(err, errors.NotImplemented), tc.IsTrue)
			c.Check(strings.Contains(err.Error(), `unknown facade type "Machiner"`), tc.IsTrue)

			// Since these are user login tests, the nonce is empty.
			err = st.Login(c.Context(), t.tag, t.password, "", nil)
			c.Assert(errors.Cause(err), tc.DeepEquals, t.err)
			c.Assert(params.ErrCode(err), tc.Equals, t.code)

			_, err = apimachiner.NewClient(st).Machine(c.Context(), names.NewMachineTag("0"))
			c.Assert(err, tc.NotNil)
			c.Check(errors.Is(err, errors.NotImplemented), tc.IsTrue)
			c.Check(strings.Contains(err.Error(), `unknown facade type "Machiner"`), tc.IsTrue)
		}()
	}
}

func (s *loginSuite) TestLoginAsDeactivatedUser(c *tc.C) {
	st := s.openAPIWithoutLogin(c)

	userTag := names.NewUserTag("charlie")
	name := user.NameFromTag(userTag)
	pass := "totally-secure-password"

	accessService := s.ControllerDomainServices(c).Access()
	_, _, err := accessService.AddUser(c.Context(), accessservice.AddUserArg{
		Name:        name,
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(pass)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = accessService.DisableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// Since these are user login tests, the nonce is empty.
	err = st.Login(c.Context(), userTag, pass, "", nil)

	// The error message should not leak that the user is disabled.
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})

	_, err = apiclient.NewClient(st, loggertesting.WrapCheckLog(c)).Status(c.Context(), nil)
	c.Assert(err, tc.NotNil)
	c.Check(errors.Is(err, errors.NotImplemented), tc.IsTrue)
	c.Check(strings.Contains(err.Error(), `unknown facade type "Client"`), tc.IsTrue)
}

func (s *loginSuite) TestLoginAsDeletedUser(c *tc.C) {
	st := s.openAPIWithoutLogin(c)

	userTag := names.NewUserTag("charlie")
	name := user.NameFromTag(userTag)
	pass := "totally-secure-password"

	accessService := s.ControllerDomainServices(c).Access()
	_, _, err := accessService.AddUser(c.Context(), accessservice.AddUserArg{
		Name:        name,
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(pass)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = accessService.RemoveUser(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// Since these are user login tests, the nonce is empty.
	err = st.Login(c.Context(), userTag, pass, "", nil)
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})

	_, err = apiclient.NewClient(st, loggertesting.WrapCheckLog(c)).Status(c.Context(), nil)
	c.Assert(err, tc.NotNil)
	c.Check(errors.Is(err, errors.NotImplemented), tc.IsTrue)
	c.Check(strings.Contains(err.Error(), `unknown facade type "Client"`), tc.IsTrue)
}

/*
func (s *loginSuite) assertAgentLogin(c *tc.C, info *api.Info, mgmtSpace *network.SpaceInfo) {
	st := s.ControllerModel(c).State()

	cfg, err := s.ControllerDomainServices(c).ControllerConfig().ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetAPIHostPorts(cfg, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Initially just the address we connect with is returned by the helper
	// because there are no APIHostPorts in state.
	connectedAddr, hostPorts := s.loginHostPorts(c, info)
	connectedAddrHost := connectedAddr.Hostname()
	connectedAddrPortString := connectedAddr.Port()
	c.Assert(err, tc.ErrorIsNil)

	connectedAddrPort, err := strconv.Atoi(connectedAddrPortString)
	c.Assert(err, tc.ErrorIsNil)

	connectedAddrHostPorts := []network.MachineHostPorts{
		network.NewMachineHostPorts(connectedAddrPort, connectedAddrHost),
	}
	c.Assert(hostPorts, tc.DeepEquals, connectedAddrHostPorts)

	// After storing APIHostPorts in state, Login should return the list
	// filtered for agents along with the address we connected to.
	server1Addresses := network.SpaceAddresses{
		network.NewSpaceAddress("server-1", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
	}
	server1Addresses[1].SpaceID = mgmtSpace.ID

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
	c.Assert(err, tc.ErrorIsNil)

	_, hostPorts = s.loginHostPorts(c, info)

	// The login method is called with a machine tag, so we expect the
	// first return slice to only have the address in the management space.
	expectedAPIHostPorts := []network.MachineHostPorts{
		{{MachineAddress: server1Addresses[1].MachineAddress, NetPort: 123}},
		{{MachineAddress: server2Addresses[0].MachineAddress, NetPort: 456}},
	}
	// Prepended as before with the connection address.
	expectedAPIHostPorts = append(connectedAddrHostPorts, expectedAPIHostPorts...)
	c.Assert(hostPorts, tc.DeepEquals, expectedAPIHostPorts)
}
*/

func (s *loginSuite) TestNoLoginPermissions(c *tc.C) {
	info := s.ControllerModelApiInfo()
	accessService := s.ControllerDomainServices(c).Access()
	password := "dummy-password"
	tag := names.NewUserTag("charliebrown")
	// Add a user with permission to log into this controller.
	_, _, err := accessService.AddUser(c.Context(), accessservice.AddUserArg{
		Name:        user.NameFromTag(tag),
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(password)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = accessService.DeletePermission(c.Context(), user.NameFromTag(tag),
		permission.ID{
			ObjectType: permission.Controller,
			Key:        s.ControllerUUID,
		})
	c.Assert(err, tc.ErrorIsNil)
	info.Password = password
	info.Tag = tag
	_, err = api.Open(c.Context(), info, fastDialOpts)
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: "permission denied",
		Code:    "unauthorized access",
	})
}

func (s *loginSuite) TestLoginValidationDuringUpgrade(c *tc.C) {
	s.WithUpgrading = true
	s.testLoginDuringMaintenance(c, func(st api.Connection) {
		var statusResult params.FullStatus
		err := st.APICall(c.Context(), "Client", clientFacadeVersion, "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, tc.ErrorIsNil)

		err = st.APICall(c.Context(), "Client", clientFacadeVersion, "", "ModelSet", params.ModelSet{}, nil)
		c.Assert(err, tc.Satisfies, params.IsCodeUpgradeInProgress)
	})
}

func (s *loginSuite) testLoginDuringMaintenance(c *tc.C, check func(api.Connection)) {
	st := s.openAPIWithoutLogin(c)
	err := st.Login(c.Context(), jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(err, tc.ErrorIsNil)

	check(st)
}

func (s *loginSuite) TestMigratedModelLoginRedirect(c *tc.C) {
	c.Skip("check login to a migrated model results in a redirect")
}

func (s *loginSuite) TestAnonymousModelLogin(c *tc.C) {
	conn := s.openAPIWithoutLogin(c)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag: names.NewUserTag(api.AnonymousUsername).String(),
	}
	err := conn.APICall(c.Context(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.UserInfo, tc.IsNil)
	c.Assert(result.ControllerTag, tc.Equals, names.NewControllerTag(s.ControllerUUID).String())
	c.Assert(result.ModelTag, tc.Equals, names.NewModelTag(s.ControllerModelUUID()).String())
	c.Assert(result.Facades, tc.DeepEquals, []params.FacadeVersions{
		{Name: "CrossModelRelations", Versions: []int{3}},
		{Name: "CrossModelSecrets", Versions: []int{1, 2}},
		{Name: "NotifyWatcher", Versions: []int{1}},
		{Name: "OfferStatusWatcher", Versions: []int{1}},
		{Name: "RelationStatusWatcher", Versions: []int{1}},
		{Name: "RelationUnitsWatcher", Versions: []int{1}},
		{Name: "RemoteRelationWatcher", Versions: []int{1}},
		{Name: "SecretsRevisionWatcher", Versions: []int{1}},
		{Name: "StringsWatcher", Versions: []int{1}},
	})
}

func (s *loginSuite) TestAnonymousControllerLogin(c *tc.C) {
	conn := s.openModelAPIWithoutLogin(c, "")

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       names.NewUserTag(api.AnonymousUsername).String(),
		ClientVersion: jujuversion.Current.String(),
	}
	err := conn.APICall(c.Context(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.UserInfo, tc.IsNil)
	c.Assert(result.ControllerTag, tc.Equals, names.NewControllerTag(s.ControllerUUID).String())
	c.Assert(result.Facades, tc.DeepEquals, []params.FacadeVersions{
		{Name: "CrossController", Versions: []int{1}},
		{Name: "NotifyWatcher", Versions: []int{1}},
	})
}

func (s *loginSuite) TestControllerModel(c *tc.C) {
	c.Skip("TODO: enable/fix once the mongo constraints code is removed completely.")

	st := s.openAPIWithoutLogin(c)

	err := st.Login(c.Context(), jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(err, tc.ErrorIsNil)

	s.assertRemoteModel(c, st, names.NewModelTag(s.ControllerModelUUID()))
}

func (s *loginSuite) TestControllerModelBadCreds(c *tc.C) {
	st := s.openAPIWithoutLogin(c)

	err := st.Login(c.Context(), jujutesting.AdminUser, "bad-password", "", nil)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) TestNonExistentModel(c *tc.C) {
	uuid, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	st := s.openModelAPIWithoutLogin(c, uuid.String())

	err = st.Login(c.Context(), jujutesting.AdminUser, jujutesting.AdminSecret, "", nil)
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: fmt.Sprintf("unknown model: %q", uuid),
		Code:    "model not found",
	})
}

func (s *loginSuite) TestInvalidModel(c *tc.C) {
	info := s.ControllerModelApiInfo()
	info.ModelTag = names.NewModelTag("rubbish")
	st, err := api.Open(c.Context(), info, fastDialOpts)
	c.Assert(err, tc.ErrorMatches, `unable to connect to API: invalid model UUID "rubbish" \(Bad Request\)`)
	c.Assert(st, tc.IsNil)
}

func (s *loginSuite) TestOtherModel(c *tc.C) {
	c.Skip("This test needs to be restored when st (*state.State) is removed from the API root.")

	userTag := names.NewUserTag("charlie")
	name := user.NameFromTag(userTag)
	pass := "shhh..."

	accessService := s.ControllerDomainServices(c).Access()

	_, _, err := accessService.AddUser(c.Context(), accessservice.AddUserArg{
		Name:        name,
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(pass)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Grant the user admin access to the default workload model.
	accessSpec := permission.AccessSpec{
		Target: permission.ID{
			ObjectType: permission.Model,
			Key:        s.DefaultModelUUID.String(),
		},
		Access: permission.AdminAccess,
	}
	_, err = accessService.CreatePermission(c.Context(), permission.UserAccessSpec{
		AccessSpec: accessSpec,
		User:       name,
	})
	c.Assert(err, tc.ErrorIsNil)

	st := s.openModelAPIWithoutLogin(c, s.DefaultModelUUID.String())

	err = st.Login(c.Context(), userTag, pass, "", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.assertRemoteModel(c, st, names.NewModelTag(s.DefaultModelUUID.String()))
}

func (s *loginSuite) TestLoginExternalUser(c *tc.C) {
	userTag := names.NewUserTag("alice@canonical.com")
	name := user.NameFromTag(userTag)

	accessService := s.ControllerDomainServices(c).Access()

	// assert the external user does not exist yet
	_, err := accessService.GetUserByName(c.Context(), name)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)

	conn := s.openAPIWithoutLogin(c)

	// setting up mock JWT authenticator
	info := jujutesting.JWTAuthInfo{
		User: userTag.Id(),
		Permissions: []jujutesting.JWTPermission{{
			ID: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
			Access: permission.LoginAccess,
		}},
	}
	data, err := json.Marshal(info)
	c.Assert(err, tc.ErrorIsNil)
	s.SetJWTAuthInfo(string(data))

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       userTag.String(),
		Token:         string(data),
		ClientVersion: jujuversion.Current.String(),
	}
	err = conn.APICall(c.Context(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.UserInfo, tc.NotNil)

	// assert the user now exists
	user, err := accessService.GetUserByName(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(user.UUID.String(), tc.NotZero)
}

func (s *loginSuite) loginLocalUser(c *tc.C, info *api.Info) (names.UserTag, params.LoginResult) {
	userTag := names.NewUserTag("charlie")
	name := user.NameFromTag(userTag)
	pass := "shhh..."

	accessService := s.ControllerDomainServices(c).Access()

	// Add a user with permission to log into this controller.
	_, _, err := accessService.AddUser(c.Context(), accessservice.AddUserArg{
		Name:        name,
		DisplayName: "Charlie Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(pass)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Grant the user admin access to the model too.
	accessSpec := permission.AccessSpec{
		Target: permission.ID{
			ObjectType: permission.Model,
			Key:        info.ModelTag.Id(),
		},
		Access: permission.AdminAccess,
	}
	_, err = accessService.CreatePermission(c.Context(), permission.UserAccessSpec{
		AccessSpec: accessSpec,
		User:       name,
	})
	c.Assert(err, tc.ErrorIsNil)

	conn := s.openAPIWithoutLogin(c)

	var result params.LoginResult
	request := &params.LoginRequest{
		AuthTag:       userTag.String(),
		Credentials:   pass,
		ClientVersion: jujuversion.Current.String(),
	}
	err = conn.APICall(c.Context(), "Admin", 3, "", "Login", request, &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.UserInfo, tc.NotNil)
	return userTag, result
}

func (s *loginSuite) TestLoginResultLocalUser(c *tc.C) {
	info := s.ControllerModelApiInfo()

	userTag, result := s.loginLocalUser(c, info)
	c.Check(result.UserInfo.Identity, tc.Equals, userTag.String())
	c.Check(result.UserInfo.ControllerAccess, tc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, tc.Equals, "admin")
}

func (s *loginSuite) TestLoginResultLocalUserEveryoneCreateOnlyNonLocal(c *tc.C) {
	info := s.ControllerModelApiInfo()

	s.setEveryoneAccess(c, permission.SuperuserAccess)

	userTag, result := s.loginLocalUser(c, info)
	c.Check(result.UserInfo.Identity, tc.Equals, userTag.String())
	c.Check(result.UserInfo.ControllerAccess, tc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, tc.Equals, "admin")
}

func (s *loginSuite) assertRemoteModel(c *tc.C, conn api.Connection, expected names.ModelTag) {
	// Look at what the api thinks it has.
	tag, ok := conn.ModelTag()
	c.Assert(ok, tc.IsTrue)
	c.Assert(tag, tc.Equals, expected)
	// Look at what the api Client thinks it has.
	client := modelconfig.NewClient(conn)

	// The code below is to verify that the API connection is operating on
	// the expected model. We make a change in state on that model, and
	// then check that it is picked up by a call to the API.

	// TODO(CodingCookieRookie): Replace commented code below with model constraints from dqlite

	// m, release := s.ApiServerSuite.Model(c, tag.Id())
	// defer release()

	expectedCons := constraints.MustParse("mem=8G")
	// err := m.State().SetModelConstraints(expectedCons)
	// c.Assert(err, tc.ErrorIsNil)

	cons, err := client.GetModelConstraints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.DeepEquals, expectedCons)
}

func (s *loginSuite) TestLoginUpdatesLastLoginAndConnection(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()

	name := usertesting.GenNewName(c, "bobbrown")
	userUUID, _, err := accessService.AddUser(c.Context(), accessservice.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
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

	_, err = accessService.CreatePermission(c.Context(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.AdminAccess,
		},
		User: name,
	})
	c.Assert(err, tc.ErrorIsNil)

	now := s.Clock.Now().UTC()

	info := s.ControllerModelApiInfo()
	info.Tag = names.NewUserTag("bobbrown")
	info.Password = "password"

	apiState, err := api.Open(c.Context(), info, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()

	// The user now has last login updated.
	user, err := accessService.GetUser(c.Context(), userUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(user.LastLogin, tc.Almost, now)

	when, err := accessService.LastModelLogin(c.Context(), name, coremodel.UUID(s.ControllerModelUUID()))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(when, tc.Almost, now)
}

func (s *loginSuite) setEveryoneAccess(c *tc.C, accessLevel permission.Access) {
	accessService := s.ControllerDomainServices(c).Access()
	err := accessService.AddExternalUser(c.Context(), permission.EveryoneUserName, "", s.AdminUserUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = accessService.UpdatePermission(c.Context(), access.UpdatePermissionArgs{
		Subject: permission.EveryoneUserName,
		Change:  permission.Grant,
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
			Access: accessLevel,
		},
	})
	c.Assert(err, tc.IsNil)
}

func assertInvalidEntityPassword(c *tc.C, err error) {
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})
}

func assertPermissionDenied(c *tc.C, err error) {
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: "permission denied",
		Code:    "unauthorized access",
	})
}
func TestMigrationSuite(t *stdtesting.T) {
	tc.Run(t, &migrationSuite{})
}

type migrationSuite struct {
	baseLoginSuite
}

func (s *migrationSuite) TestExportingModel(c *tc.C) {
	c.Skip(`check that a model that is being exported can be logged in to but
		unabled to mutate it, such as removing a machine`)
}

type loginV3Suite struct {
	baseLoginSuite
}

func TestLoginV3Suite(t *stdtesting.T) {
	tc.Run(t, &loginV3Suite{})
}

func (s *loginV3Suite) TestClientLoginToModel(c *tc.C) {
	apiState := s.OpenControllerModelAPI(c)
	client := modelconfig.NewClient(apiState)
	_, err := client.GetModelConstraints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loginV3Suite) TestClientLoginToController(c *tc.C) {
	apiState := s.OpenControllerAPI(c)
	client := machineclient.NewClient(apiState)
	_, err := client.RetryProvisioning(c.Context(), false, names.NewMachineTag("machine-0"))
	c.Assert(errors.Cause(err), tc.DeepEquals, &rpc.RequestError{
		Message: `facade "MachineManager" not supported for controller API connection`,
		Code:    "not supported",
	})
}

func (s *loginV3Suite) TestClientLoginToControllerNoAccessToControllerModel(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()
	name := usertesting.GenNewName(c, "bobbrown")
	uuid, _, err := accessService.AddUser(c.Context(), accessservice.AddUserArg{
		Name:        name,
		DisplayName: "Bob Brown",
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

	now := s.Clock.Now().UTC().Truncate(time.Second)

	s.OpenControllerAPIAs(c, names.NewUserTag(name.Name()), "password")

	user, err := accessService.GetUser(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(user.LastLogin, tc.Not(tc.Before), now)
}

func (s *loginV3Suite) TestClientLoginToRootOldClient(c *tc.C) {
	info := s.ControllerModelApiInfo()
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	info.SkipLogin = true
	apiState, err := api.Open(c.Context(), info, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)

	err = apiState.APICall(c.Context(), "Admin", 2, "", "Login", struct{}{}, nil)
	c.Assert(err, tc.ErrorMatches, ".*this version of Juju does not support login from old clients.*")
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
