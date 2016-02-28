// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type baseLoginSuite struct {
	jujutesting.JujuConnSuite
	setAdminApi func(*apiserver.Server)
}

type loginSuite struct {
	baseLoginSuite
}

var _ = gc.Suite(&loginSuite{
	baseLoginSuite{
		setAdminApi: func(srv *apiserver.Server) {
			apiserver.SetAdminApiVersions(srv, 3)
		},
	},
})

func (s *baseLoginSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
}

func (s *baseLoginSuite) setupServer(c *gc.C) (api.Connection, func()) {
	return s.setupServerForEnvironment(c, s.State.ModelTag())
}

func (s *baseLoginSuite) setupServerForEnvironment(c *gc.C, modelTag names.ModelTag) (api.Connection, func()) {
	info, cleanup := s.setupServerForEnvironmentWithValidator(c, modelTag, nil)
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	return st, func() {
		st.Close()
		cleanup()
	}
}

func (s *baseLoginSuite) setupMachineAndServer(c *gc.C) (*api.Info, func()) {
	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})
	info, cleanup := s.setupServerWithValidator(c, nil)
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	return info, cleanup
}

func (s *loginSuite) TestLoginWithInvalidTag(c *gc.C) {
	info := s.APIInfo(c)
	info.Tag = nil
	info.Password = ""
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	request := &params.LoginRequest{
		AuthTag:     "bar",
		Credentials: "password",
	}

	var response params.LoginResult
	err = st.APICall("Admin", 3, "", "Login", request, &response)
	c.Assert(err, gc.ErrorMatches, `.*"bar" is not a valid tag.*`)
}

func (s *loginSuite) TestBadLogin(c *gc.C) {
	// Start our own server so we can control when the first login
	// happens. Otherwise in JujuConnSuite.SetUpTest api.Open is
	// called with user-admin permissions automatically.
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	adminUser := s.AdminUserTag(c)

	for i, t := range []struct {
		tag      names.Tag
		password string
		err      error
		code     string
	}{{
		tag:      adminUser,
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
		// Note that Open does not log in if the tag and password
		// are empty. This allows us to test operations on the connection
		// before calling Login, which we could not do if Open
		// always logged in.
		info.Tag = nil
		info.Password = ""
		func() {
			st, err := api.Open(info, fastDialOpts)
			c.Assert(err, jc.ErrorIsNil)
			defer st.Close()

			_, err = st.Machiner().Machine(names.NewMachineTag("0"))
			c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
				Message: `unknown object type "Machiner"`,
				Code:    "not implemented",
			})

			// Since these are user login tests, the nonce is empty.
			err = st.Login(t.tag, t.password, "")
			c.Assert(errors.Cause(err), gc.DeepEquals, t.err)
			c.Assert(params.ErrCode(err), gc.Equals, t.code)

			_, err = st.Machiner().Machine(names.NewMachineTag("0"))
			c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
				Message: `unknown object type "Machiner"`,
				Code:    "not implemented",
			})
		}()
	}
}

func (s *loginSuite) TestLoginAsDeactivatedUser(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info.Tag = nil
	info.Password = ""
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	password := "password"
	u := s.Factory.MakeUser(c, &factory.UserParams{Password: password, Disabled: true})

	_, err = st.Client().Status([]string{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Client"`,
		Code:    "not implemented",
	})

	// Since these are user login tests, the nonce is empty.
	err = st.Login(u.Tag(), password, "")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})

	_, err = st.Client().Status([]string{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Client"`,
		Code:    "not implemented",
	})
}

func (s *baseLoginSuite) runLoginSetsLogIdentifier(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"

	apiConn, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer apiConn.Close()

	apiMachine, err := apiConn.Machiner().Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine.Tag(), gc.Equals, machine.Tag())
}

func (s *loginSuite) TestLoginAddrs(c *gc.C) {
	info, cleanup := s.setupMachineAndServer(c)
	defer cleanup()

	err := s.State.SetAPIHostPorts(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Initially just the address we connect with is returned,
	// despite there being no APIHostPorts in state.
	connectedAddr, hostPorts := s.loginHostPorts(c, info)
	connectedAddrHost, connectedAddrPortString, err := net.SplitHostPort(connectedAddr)
	c.Assert(err, jc.ErrorIsNil)
	connectedAddrPort, err := strconv.Atoi(connectedAddrPortString)
	c.Assert(err, jc.ErrorIsNil)
	connectedAddrHostPorts := [][]network.HostPort{
		network.NewHostPorts(connectedAddrPort, connectedAddrHost),
	}
	c.Assert(hostPorts, gc.DeepEquals, connectedAddrHostPorts)

	// After storing APIHostPorts in state, Login should store
	// all of them and the address we connected with.
	server1Addresses := []network.Address{{
		Value: "server-1",
		Type:  network.HostName,
		Scope: network.ScopePublic,
	}, {
		Value:       "10.0.0.1",
		Type:        network.IPv4Address,
		NetworkName: "internal",
		Scope:       network.ScopeCloudLocal,
	}}
	server2Addresses := []network.Address{{
		Value:       "::1",
		Type:        network.IPv6Address,
		NetworkName: "loopback",
		Scope:       network.ScopeMachineLocal,
	}}
	stateAPIHostPorts := [][]network.HostPort{
		network.AddressesWithPort(server1Addresses, 123),
		network.AddressesWithPort(server2Addresses, 456),
	}
	err = s.State.SetAPIHostPorts(stateAPIHostPorts)
	c.Assert(err, jc.ErrorIsNil)
	_, hostPorts = s.loginHostPorts(c, info)
	// Now that we connected, we add the other stateAPIHostPorts. However,
	// the one we connected to comes first.
	stateAPIHostPorts = append(connectedAddrHostPorts, stateAPIHostPorts...)
	c.Assert(hostPorts, gc.DeepEquals, stateAPIHostPorts)
}

func (s *baseLoginSuite) loginHostPorts(c *gc.C, info *api.Info) (connectedAddr string, hostPorts [][]network.HostPort) {
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	return st.Addr(), st.APIHostPorts()
}

func startNLogins(c *gc.C, n int, info *api.Info) (chan error, *sync.WaitGroup) {
	errResults := make(chan error, 100)
	var doneWG sync.WaitGroup
	var startedWG sync.WaitGroup
	c.Logf("starting %d concurrent logins to %v", n, info.Addrs)
	for i := 0; i < n; i++ {
		i := i
		c.Logf("starting login request %d", i)
		startedWG.Add(1)
		doneWG.Add(1)
		go func() {
			c.Logf("started login %d", i)
			startedWG.Done()
			st, err := api.Open(info, fastDialOpts)
			errResults <- err
			if err == nil {
				st.Close()
			}
			doneWG.Done()
			c.Logf("finished login %d: %v", i, err)
		}()
	}
	startedWG.Wait()
	return errResults, &doneWG
}

func (s *loginSuite) TestDelayLogins(c *gc.C) {
	info, cleanup := s.setupMachineAndServer(c)
	defer cleanup()
	delayChan, cleanup := apiserver.DelayLogins()
	defer cleanup()

	// numConcurrentLogins is how many logins will fire off simultaneously.
	// It doesn't really matter, as long as it is less than LoginRateLimit
	const numConcurrentLogins = 5
	c.Assert(numConcurrentLogins, jc.LessThan, apiserver.LoginRateLimit)
	// Trigger a bunch of login requests
	errResults, wg := startNLogins(c, numConcurrentLogins, info)
	select {
	case err := <-errResults:
		c.Fatalf("we should not have gotten any logins yet: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	// Allow one login to proceed
	c.Logf("letting one login through")
	select {
	case delayChan <- struct{}{}:
	default:
		c.Fatalf("we should have been able to unblock a login")
	}
	select {
	case err := <-errResults:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for Login to finish")
	}
	c.Logf("checking no other logins succeeded")
	// It should have only let 1 login through
	select {
	case err := <-errResults:
		c.Fatalf("we should not have gotten more logins: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	// Now allow the rest of the logins to proceed
	c.Logf("letting %d logins through", numConcurrentLogins-1)
	for i := 0; i < numConcurrentLogins-1; i++ {
		delayChan <- struct{}{}
	}
	c.Logf("waiting for Logins to finish")
	wg.Wait()
	close(errResults)
	successCount := 0
	for err := range errResults {
		c.Check(err, jc.ErrorIsNil)
		if err == nil {
			successCount += 1
		}
	}
	// All the logins should succeed, they were just delayed after
	// connecting.
	c.Check(successCount, gc.Equals, numConcurrentLogins-1)
	c.Logf("done")
}

func (s *loginSuite) TestLoginRateLimited(c *gc.C) {
	info, cleanup := s.setupMachineAndServer(c)
	defer cleanup()
	delayChan, cleanup := apiserver.DelayLogins()
	defer cleanup()

	// Start enough concurrent Login requests so that we max out our
	// LoginRateLimit. Do one extra so we know we are in overload
	errResults, wg := startNLogins(c, apiserver.LoginRateLimit+1, info)
	select {
	case err := <-errResults:
		c.Check(err, jc.Satisfies, params.IsCodeTryAgain)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for login to get rejected.")
	}

	// Let one request through, we should see that it succeeds without
	// error, and then be able to start a new request, but it will block
	delayChan <- struct{}{}
	select {
	case err := <-errResults:
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out expecting one login to succeed")
	}
	chOne := make(chan error, 1)
	wg.Add(1)
	go func() {
		st, err := api.Open(info, fastDialOpts)
		chOne <- err
		if err == nil {
			st.Close()
		}
		wg.Done()
	}()
	select {
	case err := <-chOne:
		c.Fatalf("the open request should not have completed: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	// Let all the logins finish. We started with LoginRateLimit, let one
	// proceed, but we issued another one, so there should be
	// LoginRateLimit logins pending.
	for i := 0; i < apiserver.LoginRateLimit; i++ {
		delayChan <- struct{}{}
	}
	wg.Wait()
	close(errResults)
	for err := range errResults {
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *loginSuite) TestUsersLoginWhileRateLimited(c *gc.C) {
	info, cleanup := s.setupMachineAndServer(c)
	defer cleanup()
	delayChan, cleanup := apiserver.DelayLogins()
	defer cleanup()

	// Start enough concurrent Login requests so that we max out our
	// LoginRateLimit. Do one extra so we know we are in overload
	machineResults, machineWG := startNLogins(c, apiserver.LoginRateLimit+1, info)
	select {
	case err := <-machineResults:
		c.Check(err, jc.Satisfies, params.IsCodeTryAgain)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for login to get rejected.")
	}

	userInfo := *info
	userInfo.Tag = s.AdminUserTag(c)
	userInfo.Password = "dummy-secret"
	userResults, userWG := startNLogins(c, apiserver.LoginRateLimit+1, &userInfo)
	// all of them should have started, and none of them in TryAgain state
	select {
	case err := <-userResults:
		c.Fatalf("we should not have gotten any logins yet: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	totalLogins := apiserver.LoginRateLimit*2 + 1
	for i := 0; i < totalLogins; i++ {
		delayChan <- struct{}{}
	}
	machineWG.Wait()
	close(machineResults)
	userWG.Wait()
	close(userResults)
	machineCount := 0
	for err := range machineResults {
		machineCount += 1
		c.Check(err, jc.ErrorIsNil)
	}
	c.Check(machineCount, gc.Equals, apiserver.LoginRateLimit)
	userCount := 0
	for err := range userResults {
		userCount += 1
		c.Check(err, jc.ErrorIsNil)
	}
	c.Check(userCount, gc.Equals, apiserver.LoginRateLimit+1)
}

func (s *loginSuite) TestUsersAreNotRateLimited(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	info.Tag = s.AdminUserTag(c)
	info.Password = "dummy-secret"
	defer cleanup()
	delayChan, cleanup := apiserver.DelayLogins()
	defer cleanup()
	// We can login more than LoginRateLimit users
	nLogins := apiserver.LoginRateLimit * 2
	errResults, wg := startNLogins(c, nLogins, info)
	select {
	case err := <-errResults:
		c.Fatalf("we should not have gotten any logins yet: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	c.Logf("letting %d logins complete", nLogins)
	for i := 0; i < nLogins; i++ {
		delayChan <- struct{}{}
	}
	c.Logf("waiting for original requests to finish")
	wg.Wait()
	close(errResults)
	for err := range errResults {
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *loginSuite) TestNonEnvironUserLoginFails(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "dummy-password", NoModelUser: true})
	info.Password = "dummy-password"
	info.Tag = user.UserTag()
	_, err := api.Open(info, fastDialOpts)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})
}

func (s *loginSuite) TestLoginValidationSuccess(c *gc.C) {
	validator := func(params.LoginRequest) error {
		return nil
	}
	checker := func(c *gc.C, loginErr error, st api.Connection) {
		c.Assert(loginErr, gc.IsNil)

		// Ensure an API call that would be restricted during
		// upgrades works after a normal login.
		err := st.APICall("Client", 1, "", "DestroyModel", nil, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	s.checkLoginWithValidator(c, validator, checker)
}

func (s *loginSuite) TestLoginValidationFail(c *gc.C) {
	validator := func(params.LoginRequest) error {
		return errors.New("Login not allowed")
	}
	checker := func(c *gc.C, loginErr error, _ api.Connection) {
		// error is wrapped in API server
		c.Assert(loginErr, gc.ErrorMatches, "Login not allowed")
	}
	s.checkLoginWithValidator(c, validator, checker)
}

func (s *loginSuite) TestLoginValidationDuringUpgrade(c *gc.C) {
	validator := func(params.LoginRequest) error {
		return apiserver.UpgradeInProgressError
	}
	checker := func(c *gc.C, loginErr error, st api.Connection) {
		c.Assert(loginErr, gc.IsNil)

		var statusResult params.FullStatus
		err := st.APICall("Client", 1, "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, jc.ErrorIsNil)

		err = st.APICall("Client", 1, "", "DestroyModel", nil, nil)
		c.Assert(err, gc.ErrorMatches, ".*upgrade in progress - Juju functionality is limited.*")
	}
	s.checkLoginWithValidator(c, validator, checker)
}

func (s *loginSuite) TestFailedLoginDuringMaintenance(c *gc.C) {
	validator := func(params.LoginRequest) error {
		return errors.New("something")
	}
	info, cleanup := s.setupServerWithValidator(c, validator)
	defer cleanup()

	checkLogin := func(tag names.Tag) {
		st := s.openAPIWithoutLogin(c, info)
		defer st.Close()
		err := st.Login(tag, "dummy-secret", "nonce")
		c.Assert(err, gc.ErrorMatches, "something")
	}
	checkLogin(names.NewUserTag("definitelywontexist"))
	checkLogin(names.NewMachineTag("99999"))
}

type validationChecker func(c *gc.C, err error, st api.Connection)

func (s *baseLoginSuite) checkLoginWithValidator(c *gc.C, validator apiserver.LoginValidator, checker validationChecker) {
	info, cleanup := s.setupServerWithValidator(c, validator)
	defer cleanup()

	st := s.openAPIWithoutLogin(c, info)
	defer st.Close()

	// Ensure not already logged in.
	_, err := st.Machiner().Machine(names.NewMachineTag("0"))
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Machiner"`,
		Code:    "not implemented",
	})

	adminUser := s.AdminUserTag(c)
	// Since these are user login tests, the nonce is empty.
	err = st.Login(adminUser, "dummy-secret", "")

	checker(c, err, st)
}

func (s *baseLoginSuite) setupServerWithValidator(c *gc.C, validator apiserver.LoginValidator) (*api.Info, func()) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	return s.setupServerForEnvironmentWithValidator(c, env.ModelTag(), validator)
}

func (s *baseLoginSuite) setupServerForEnvironmentWithValidator(c *gc.C, modelTag names.ModelTag, validator apiserver.LoginValidator) (*api.Info, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(
		s.State,
		listener,
		apiserver.ServerConfig{
			Cert:      []byte(coretesting.ServerCert),
			Key:       []byte(coretesting.ServerKey),
			Validator: validator,
			Tag:       names.NewMachineTag("0"),
			LogDir:    c.MkDir(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.setAdminApi, gc.NotNil)
	s.setAdminApi(srv)
	info := &api.Info{
		Tag:      nil,
		Password: "",
		ModelTag: modelTag,
		Addrs:    []string{srv.Addr().String()},
		CACert:   coretesting.CACert,
	}
	return info, func() {
		err := srv.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *baseLoginSuite) openAPIWithoutLogin(c *gc.C, info *api.Info) api.Connection {
	info.Tag = nil
	info.Password = ""
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	return st
}

func (s *loginSuite) TestControllerModel(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	c.Assert(info.ModelTag, gc.Equals, s.State.ModelTag())
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	adminUser := s.AdminUserTag(c)
	err = st.Login(adminUser, "dummy-secret", "")
	c.Assert(err, jc.ErrorIsNil)

	s.assertRemoteEnvironment(c, st, s.State.ModelTag())
}

func (s *loginSuite) TestControllerModelBadCreds(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	c.Assert(info.ModelTag, gc.Equals, s.State.ModelTag())
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	adminUser := s.AdminUserTag(c)
	err = st.Login(adminUser, "bad-password", "")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `invalid entity name or password`,
		Code:    "unauthorized access",
	})
}

func (s *loginSuite) TestNonExistentEnvironment(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = names.NewModelTag(uuid.String())
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	adminUser := s.AdminUserTag(c)
	err = st.Login(adminUser, "dummy-secret", "")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: fmt.Sprintf("unknown model: %q", uuid),
		Code:    "not found",
	})
}

func (s *loginSuite) TestInvalidEnvironment(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	info.ModelTag = names.NewModelTag("rubbish")
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	adminUser := s.AdminUserTag(c)
	err = st.Login(adminUser, "dummy-secret", "")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown model: "rubbish"`,
		Code:    "not found",
	})
}

func (s *loginSuite) TestOtherEnvironment(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	envOwner := s.Factory.MakeUser(c, nil)
	envState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: envOwner.UserTag(),
	})
	defer envState.Close()
	info.ModelTag = envState.ModelTag()
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = st.Login(envOwner.UserTag(), "password", "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertRemoteEnvironment(c, st, envState.ModelTag())
}

func (s *loginSuite) TestMachineLoginOtherEnvironment(c *gc.C) {
	// User credentials are checked against a global user list.
	// Machine credentials are checked against environment specific
	// machines, so this makes sure that the credential checking is
	// using the correct state connection.
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	envOwner := s.Factory.MakeUser(c, nil)
	envState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: envOwner.UserTag(),
		ConfigAttrs: map[string]interface{}{
			"controller": false,
		},
		Prepare: true,
	})
	defer envState.Close()

	f2 := factory.NewFactory(envState)
	machine, password := f2.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "nonce",
	})

	info.ModelTag = envState.ModelTag()
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = st.Login(machine.Tag(), password, "nonce")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherEnvironmentFromController(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})

	envState := s.Factory.MakeModel(c, nil)
	defer envState.Close()
	info.ModelTag = envState.ModelTag()
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = st.Login(machine.Tag(), password, "nonce")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherEnvironmentWhenNotController(c *gc.C) {
	info, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	machine, password := s.Factory.MakeMachineReturningPassword(c, nil)

	envState := s.Factory.MakeModel(c, nil)
	defer envState.Close()
	info.ModelTag = envState.ModelTag()
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = st.Login(machine.Tag(), password, "nonce")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})
}

func (s *loginSuite) assertRemoteEnvironment(c *gc.C, st api.Connection, expected names.ModelTag) {
	// Look at what the api thinks it has.
	tag, err := st.ModelTag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.Equals, expected)
	// Look at what the api Client thinks it has.
	client := st.Client()

	// ModelUUID looks at the env tag on the api state connection.
	c.Assert(client.ModelUUID(), gc.Equals, expected.Id())

	// ModelInfo calls a remote method that looks up the environment.
	info, err := client.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.UUID, gc.Equals, expected.Id())
}

func (s *loginSuite) TestLoginUpdatesLastLoginAndConnection(c *gc.C) {
	_, cleanup := s.setupServerWithValidator(c, nil)
	defer cleanup()

	// Since the login and connection times truncate time to the second,
	// we need to make sure our start time is just before now.
	startTime := time.Now().Add(-time.Second)

	password := "shhh..."
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Password: password,
	})

	info := s.APIInfo(c)
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
	c.Assert(lastLogin, gc.NotNil)
	c.Assert(lastLogin.After(startTime), jc.IsTrue)

	// The env user is also updated.
	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := modelUser.LastConnection()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when, gc.NotNil)
	c.Assert(when.After(startTime), jc.IsTrue)
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
}

func (s *macaroonLoginSuite) TestLoginToController(c *gc.C) {
	// Note that currently we cannot use macaroon auth
	// to log into the controller rather than an environment
	// because there's no place to store the fact that
	// a given external user is allowed access to the controller.
	s.DischargerLogin = func() string {
		return "test@somewhere"
	}
	info := s.APIInfo(c)

	// Zero the environment tag so that we log into the controller
	// not the environment.
	info.ModelTag = names.ModelTag{}

	client, err := api.Open(info, api.DialOpts{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})
	c.Assert(client, gc.Equals, nil)
}

func (s *macaroonLoginSuite) TestLoginToEnvironmentSuccess(c *gc.C) {
	s.AddModelUser(c, "test@somewhere")
	s.DischargerLogin = func() string {
		return "test@somewhere"
	}
	client, err := api.Open(s.APIInfo(c), api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	client.Close()
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
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "invalid entity name or password",
		Code:    "unauthorized access",
	})
	c.Assert(client, gc.Equals, nil)
}
