// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/machiner"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type baseLoginSuite struct {
	jujutesting.JujuConnSuite
	pool *state.StatePool
}

type loginSuite struct {
	baseLoginSuite
}

var _ = gc.Suite(&loginSuite{})

func (s *baseLoginSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	s.pool = state.NewStatePool(s.State)
	s.AddCleanup(func(*gc.C) { s.pool.Close() })
}

func (s *baseLoginSuite) newMachineAndServer(c *gc.C) (*api.Info, *apiserver.Server) {
	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})
	info, srv := newServer(c, s.pool)
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	return info, srv
}

func (s *loginSuite) TestLoginWithInvalidTag(c *gc.C) {
	info := s.APIInfo(c)
	info.Tag = nil
	info.Password = ""
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
	// Start our own server so we can control when the first login
	// happens. Otherwise in JujuConnSuite.SetUpTest api.Open is
	// called with user-admin permissions automatically.
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()

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
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()

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
	assertInvalidEntityPassword(c, err)

	_, err = st.Client().Status([]string{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Client"`,
		Code:    "not implemented",
	})
}

func (s *baseLoginSuite) runLoginSetsLogIdentifier(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	machine, password := s.Factory.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"

	apiConn, err := api.Open(info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer apiConn.Close()

	apiMachine, err := apimachiner.NewState(apiConn).Machine(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine.Tag(), gc.Equals, machine.Tag())
}

func (s *loginSuite) TestLoginAddrs(c *gc.C) {
	info, srv := s.newMachineAndServer(c)
	defer assertStop(c, srv)

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
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}
	server2Addresses := []network.Address{{
		Value: "::1",
		Type:  network.IPv6Address,
		Scope: network.ScopeMachineLocal,
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
	info, srv := s.newMachineAndServer(c)
	defer assertStop(c, srv)
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
	info, srv := s.newMachineAndServer(c)
	defer assertStop(c, srv)
	delayChan, cleanup := apiserver.DelayLogins()
	defer cleanup()

	// Start enough concurrent Login requests so that we max out our
	// LoginRateLimit. Do one extra so we know we are in overload
	errResults, wg := startNLogins(c, apiserver.LoginRateLimit+1, info)
	select {
	case err := <-errResults:
		c.Check(err, jc.Satisfies, params.IsCodeTryAgain)
	case <-time.After(apiserver.LoginRetyPause + coretesting.LongWait):
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
	info, srv := s.newMachineAndServer(c)
	defer assertStop(c, srv)
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
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	info.Tag = s.AdminUserTag(c)
	info.Password = "dummy-secret"
	info.ModelTag = s.State.ModelTag()

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

func (s *loginSuite) TestNonModelUserLoginFails(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "dummy-password", NoModelUser: true})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	err := s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	info.Password = "dummy-password"
	info.Tag = user.UserTag()
	_, err = api.Open(info, fastDialOpts)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) TestLoginValidationSuccess(c *gc.C) {
	validator := func(params.LoginRequest) error {
		return nil
	}
	checker := func(c *gc.C, loginErr error, st api.Connection) {
		c.Assert(loginErr, gc.IsNil)

		// Ensure an API call that would be restricted during
		// upgrades works after a normal login.
		err := st.APICall("Client", 1, "", "ModelSet", params.ModelSet{}, nil)
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
		return params.UpgradeInProgressError
	}
	checker := func(c *gc.C, loginErr error, st api.Connection) {
		c.Assert(loginErr, gc.IsNil)

		var statusResult params.FullStatus
		err := st.APICall("Client", 1, "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, jc.ErrorIsNil)

		err = st.APICall("Client", 1, "", "ModelSet", params.ModelSet{}, nil)
		c.Assert(err, jc.Satisfies, params.IsCodeUpgradeInProgress)
	}
	s.checkLoginWithValidator(c, validator, checker)
}

func (s *loginSuite) TestFailedLoginDuringMaintenance(c *gc.C) {
	cfg := defaultServerConfig(c)
	cfg.Validator = func(params.LoginRequest) error {
		return errors.New("something")
	}
	info, srv := newServerWithConfig(c, s.pool, cfg)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()

	checkLogin := func(tag names.Tag) {
		st := s.openAPIWithoutLogin(c, info)
		err := st.Login(tag, "dummy-secret", "nonce", nil)
		c.Assert(err, gc.ErrorMatches, "something")
	}
	checkLogin(names.NewUserTag("definitelywontexist"))
	checkLogin(names.NewMachineTag("99999"))
}

type validationChecker func(c *gc.C, err error, st api.Connection)

func (s *baseLoginSuite) checkLoginWithValidator(c *gc.C, validator apiserver.LoginValidator, checker validationChecker) {
	cfg := defaultServerConfig(c)
	cfg.Validator = validator
	info, srv := newServerWithConfig(c, s.pool, cfg)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()

	st := s.openAPIWithoutLogin(c, info)

	// Ensure not already logged in.
	_, err := apimachiner.NewState(st).Machine(names.NewMachineTag("0"))
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown object type "Machiner"`,
		Code:    "not implemented",
	})

	adminUser := s.AdminUserTag(c)
	// Since these are user login tests, the nonce is empty.
	err = st.Login(adminUser, "dummy-secret", "", nil)

	checker(c, err, st)
}

func (s *baseLoginSuite) openAPIWithoutLogin(c *gc.C, info0 *api.Info) api.Connection {
	info := *info0
	info.Tag = nil
	info.Password = ""
	info.SkipLogin = true
	st, err := api.Open(&info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	return st
}

func (s *loginSuite) TestControllerModel(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	info.ModelTag = s.State.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	adminUser := s.AdminUserTag(c)
	err := st.Login(adminUser, "dummy-secret", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertRemoteModel(c, st, s.State.ModelTag())
}

func (s *loginSuite) TestControllerModelBadCreds(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	info.ModelTag = s.State.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	adminUser := s.AdminUserTag(c)
	err := st.Login(adminUser, "bad-password", "", nil)
	assertInvalidEntityPassword(c, err)
}

func (s *loginSuite) TestNonExistentModel(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	info.ModelTag = names.NewModelTag(uuid.String())
	st := s.openAPIWithoutLogin(c, info)

	adminUser := s.AdminUserTag(c)
	err = st.Login(adminUser, "dummy-secret", "", nil)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: fmt.Sprintf("unknown model: %q", uuid),
		Code:    "model not found",
	})
}

func (s *loginSuite) TestInvalidModel(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)
	info.ModelTag = names.NewModelTag("rubbish")

	st := s.openAPIWithoutLogin(c, info)

	adminUser := s.AdminUserTag(c)
	err := st.Login(adminUser, "dummy-secret", "", nil)
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown model: "rubbish"`,
		Code:    "model not found",
	})
}

func (s *loginSuite) TestOtherModel(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	envOwner := s.Factory.MakeUser(c, nil)
	envState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: envOwner.UserTag(),
	})
	defer envState.Close()
	info.ModelTag = envState.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	err := st.Login(envOwner.UserTag(), "password", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertRemoteModel(c, st, envState.ModelTag())
}

func (s *loginSuite) TestMachineLoginOtherModel(c *gc.C) {
	// User credentials are checked against a global user list.
	// Machine credentials are checked against environment specific
	// machines, so this makes sure that the credential checking is
	// using the correct state connection.
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	envOwner := s.Factory.MakeUser(c, nil)
	envState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: envOwner.UserTag(),
		ConfigAttrs: map[string]interface{}{
			"controller": false,
		},
	})
	defer envState.Close()

	f2 := factory.NewFactory(envState)
	machine, password := f2.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "nonce",
	})

	info.ModelTag = envState.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	err := st.Login(machine.Tag(), password, "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestMachineLoginOtherModelNotProvisioned(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	envOwner := s.Factory.MakeUser(c, nil)
	envState := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: envOwner.UserTag(),
		ConfigAttrs: map[string]interface{}{
			"controller": false,
		},
	})
	defer envState.Close()

	f2 := factory.NewFactory(envState)
	machine, password := f2.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})

	info.ModelTag = envState.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	// If the agent attempts Login before the provisioner has recorded
	// the machine's nonce in state, then the agent should get back an
	// error with code "not provisioned".
	err := st.Login(machine.Tag(), password, "nonce", nil)
	c.Assert(err, gc.ErrorMatches, `machine 0 not provisioned \(not provisioned\)`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)
}

func (s *loginSuite) TestOtherEnvironmentFromController(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})

	envState := s.Factory.MakeModel(c, nil)
	defer envState.Close()
	info.ModelTag = envState.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	err := st.Login(machine.Tag(), password, "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherEnvironmentFromControllerOtherNotProvisioned(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	managerMachine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})

	// Create a hosted model with an unprovisioned machine that has the
	// same tag as the manager machine.
	hostedModelState := s.Factory.MakeModel(c, nil)
	defer hostedModelState.Close()
	f2 := factory.NewFactory(hostedModelState)
	workloadMachine, _ := f2.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})
	c.Assert(managerMachine.Tag(), gc.Equals, workloadMachine.Tag())

	info.ModelTag = hostedModelState.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	// The fact that the machine with the same tag in the hosted
	// model is unprovisioned should not cause the login to fail
	// with "not provisioned", because the passwords don't match.
	err := st.Login(managerMachine.Tag(), password, "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginSuite) TestOtherEnvironmentWhenNotController(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)

	machine, password := s.Factory.MakeMachineReturningPassword(c, nil)

	envState := s.Factory.MakeModel(c, nil)
	defer envState.Close()
	info.ModelTag = envState.ModelTag()
	st := s.openAPIWithoutLogin(c, info)

	err := st.Login(machine.Tag(), password, "nonce", nil)
	assertPermissionDenied(c, err)
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
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()

	user, result := s.loginLocalUser(c, info)
	c.Check(result.UserInfo.Identity, gc.Equals, user.Tag().String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "admin")
}

func (s *loginSuite) TestLoginResultLocalUserEveryoneCreateOnlyNonLocal(c *gc.C) {
	info, srv := newServer(c, s.pool)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()

	setEveryoneAccess(c, s.State, s.AdminUserTag(c), permission.AddModelAccess)

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

	// ModelUUID looks at the env tag on the api state connection.
	uuid, ok := client.ModelUUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, expected.Id())

	// The code below is to verify that the API connection is operating on
	// the expected model. We make a change in state on that model, and
	// then check that it is picked up by a call to the API.

	st, err := s.State.ForModel(tag)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	expectedCons := constraints.MustParse("mem=8G")
	err = st.SetModelConstraints(expectedCons)
	c.Assert(err, jc.ErrorIsNil)

	cons, err := client.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, expectedCons)
}

func (s *loginSuite) TestLoginUpdatesLastLoginAndConnection(c *gc.C) {
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
	modelUser, err := s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := s.State.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when, gc.NotNil)
	c.Assert(when.After(startTime), jc.IsTrue)
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
	pool *state.StatePool
}

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.MacaroonSuite.SetUpTest(c)
	s.pool = state.NewStatePool(s.State)
	s.AddCleanup(func(*gc.C) { s.pool.Close() })
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
	assertInvalidEntityPassword(c, err)
	c.Assert(client, gc.Equals, nil)
}

func (s *macaroonLoginSuite) login(c *gc.C, info *api.Info) (params.LoginResult, error) {
	info.SkipLogin = true

	cookieJar := apitesting.NewClearableCookieJar()

	client := s.OpenAPI(c, info, cookieJar)
	defer client.Close()

	var (
		// Remote users start with an empty login request.
		request params.LoginRequest
		result  params.LoginResult
	)
	err := client.APICall("Admin", 3, "", "Login", &request, &result)
	c.Assert(err, jc.ErrorIsNil)

	cookieURL := &url.URL{
		Scheme: "https",
		Host:   "localhost",
		Path:   "/",
	}

	bakeryClient := httpbakery.NewClient()

	err = bakeryClient.HandleError(cookieURL, &httpbakery.Error{
		Message: result.DischargeRequiredReason,
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     result.DischargeRequired,
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
}

func (s *macaroonLoginSuite) TestRemoteUserLoginToControllerAddModelAccess(c *gc.C) {
	setEveryoneAccess(c, s.State, s.AdminUserTag(c), permission.AddModelAccess)
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
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "add-model")
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
	cfg := defaultServerConfig(c)
	cfg.AllowModelAccess = allowModelAccess

	info, srv := newServerWithConfig(c, s.pool, cfg)
	defer assertStop(c, srv)
	info.ModelTag = s.State.ModelTag()

	// If we have a remote user which has explict model access, but neither
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

	_, err := s.login(c, info)
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
	s.AddControllerUser(c, remoteUser, permission.AddModelAccess)

	s.DischargerLogin = func() string {
		return remoteUser
	}
	info := s.APIInfo(c)

	result, err := s.login(c, info)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result.UserInfo, gc.NotNil)
	c.Check(result.UserInfo.Identity, gc.Equals, remoteUserTag.String())
	c.Check(result.UserInfo.ControllerAccess, gc.Equals, "add-model")
	c.Check(result.UserInfo.ModelAccess, gc.Equals, "write")
}

func (s *macaroonLoginSuite) TestLoginToEnvironmentSuccess(c *gc.C) {
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
