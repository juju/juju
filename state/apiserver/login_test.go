// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type loginSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&loginSuite{})

var badLoginTests = []struct {
	tag      string
	password string
	err      string
	code     string
}{{
	tag:      "user-admin",
	password: "wrong password",
	err:      "invalid entity name or password",
	code:     params.CodeUnauthorized,
}, {
	tag:      "user-foo",
	password: "password",
	err:      "invalid entity name or password",
	code:     params.CodeUnauthorized,
}, {
	tag:      "bar",
	password: "password",
	err:      `"bar" is not a valid tag`,
}}

func (s *loginSuite) setupServer(c *gc.C) (*api.Info, func()) {
	return s.setupServerWithValidator(c, nil)
}

func (s *loginSuite) setupMachineAndServer(c *gc.C) (*api.Info, func()) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	info, cleanup := s.setupServer(c)
	info.Tag = machine.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"
	return info, cleanup
}

func (s *loginSuite) TestBadLogin(c *gc.C) {
	// Start our own server so we can control when the first login
	// happens. Otherwise in JujuConnSuite.SetUpTest api.Open is
	// called with user-admin permissions automatically.
	info, cleanup := s.setupServer(c)
	defer cleanup()

	for i, t := range badLoginTests {
		c.Logf("test %d; entity %q; password %q", i, t.tag, t.password)
		// Note that Open does not log in if the tag and password
		// are empty. This allows us to test operations on the connection
		// before calling Login, which we could not do if Open
		// always logged in.
		info.Tag = nil
		info.Password = ""
		func() {
			st, err := api.Open(info, fastDialOpts)
			c.Assert(err, gc.IsNil)
			defer st.Close()

			_, err = st.Machiner().Machine(names.NewMachineTag("0"))
			c.Assert(err, gc.ErrorMatches, `unknown object type "Machiner"`)

			// Since these are user login tests, the nonce is empty.
			err = st.Login(t.tag, t.password, "")
			c.Assert(err, gc.ErrorMatches, t.err)
			c.Assert(params.ErrCode(err), gc.Equals, t.code)

			_, err = st.Machiner().Machine(names.NewMachineTag("0"))
			c.Assert(err, gc.ErrorMatches, `unknown object type "Machiner"`)
		}()
	}
}

func (s *loginSuite) TestLoginAsDeactivatedUser(c *gc.C) {
	info, cleanup := s.setupServer(c)
	defer cleanup()

	info.Tag = nil
	info.Password = ""
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer st.Close()
	password := "password"
	u := s.Factory.MakeUser(factory.UserParams{Password: password})
	err = u.Deactivate()
	c.Assert(err, gc.IsNil)

	_, err = st.Client().Status([]string{})
	c.Assert(err, gc.ErrorMatches, `unknown object type "Client"`)

	// Since these are user login tests, the nonce is empty.
	err = st.Login(u.Tag().String(), password, "")
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")

	_, err = st.Client().Status([]string{})
	c.Assert(err, gc.ErrorMatches, `unknown object type "Client"`)
}

func (s *loginSuite) TestLoginSetsLogIdentifier(c *gc.C) {
	info, cleanup := s.setupServer(c)
	defer cleanup()

	machineInState, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machineInState.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machineInState.SetPassword(password)
	c.Assert(err, gc.IsNil)
	c.Assert(machineInState.Tag(), gc.Equals, names.NewMachineTag("0"))

	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("login-tester", &tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("login-tester")

	// TODO(dfc) this should be a Tag
	info.Tag = machineInState.Tag()
	info.Password = password
	info.Nonce = "fake_nonce"

	apiConn, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer apiConn.Close()
	apiMachine, err := apiConn.Machiner().Machine(machineInState.Tag().(names.MachineTag))
	c.Assert(err, gc.IsNil)
	c.Assert(apiMachine.Tag(), gc.Equals, machineInState.Tag())

	c.Assert(tw.Log(), jc.LogMatches, []string{
		`<- \[[0-9A-F]+\] <unknown> {"RequestId":1,"Type":"Admin","Request":"Login",` +
			`"Params":{"AuthTag":"machine-0","Password":"[^"]*","Nonce":"fake_nonce"}` +
			`}`,
		// Now that we are logged in, we see the entity's tag
		// [0-9.umns] is to handle timestamps that are ns, us, ms, or s
		// long, though we expect it to be in the 'ms' range.
		`-> \[[0-9A-F]+\] machine-0 [0-9.]+[umn]?s {"RequestId":1,"Response":.*} Admin\[""\].Login`,
		`<- \[[0-9A-F]+\] machine-0 {"RequestId":2,"Type":"Machiner","Request":"Life","Params":{"Entities":\[{"Tag":"machine-0"}\]}}`,
		`-> \[[0-9A-F]+\] machine-0 [0-9.umns]+ {"RequestId":2,"Response":{"Results":\[{"Life":"alive","Error":null}\]}} Machiner\[""\]\.Life`,
	})
}

func (s *loginSuite) TestLoginAddrs(c *gc.C) {
	info, cleanup := s.setupMachineAndServer(c)
	defer cleanup()

	// Initially just the address we connect with is returned,
	// despite there being no APIHostPorts in state.
	connectedAddr, hostPorts := s.loginHostPorts(c, info)
	connectedAddrHost, connectedAddrPortString, err := net.SplitHostPort(connectedAddr)
	c.Assert(err, gc.IsNil)
	connectedAddrPort, err := strconv.Atoi(connectedAddrPortString)
	c.Assert(err, gc.IsNil)
	connectedAddrHostPorts := [][]network.HostPort{
		[]network.HostPort{{
			network.NewAddress(connectedAddrHost, network.ScopeUnknown),
			connectedAddrPort,
		}},
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
	c.Assert(err, gc.IsNil)
	connectedAddr, hostPorts = s.loginHostPorts(c, info)
	// Now that we connected, we add the other stateAPIHostPorts. However,
	// the one we connected to comes first.
	stateAPIHostPorts = append(connectedAddrHostPorts, stateAPIHostPorts...)
	c.Assert(hostPorts, gc.DeepEquals, stateAPIHostPorts)
}

func (s *loginSuite) loginHostPorts(c *gc.C, info *api.Info) (connectedAddr string, hostPorts [][]network.HostPort) {
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
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
		c.Check(err, gc.IsNil)
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
		c.Check(err, gc.IsNil)
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
	// LoginRateLimit
	errResults, wg := startNLogins(c, apiserver.LoginRateLimit, info)
	// All of them should have started, none of them should have succeeded
	// (or failed) yet
	select {
	case err := <-errResults:
		c.Fatalf("we should not have gotten any logins yet: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	// We now have a bunch of pending Login requests, the next login
	// request should be immediately bounced
	_, err := api.Open(info, fastDialOpts)
	c.Check(err, gc.ErrorMatches, "try again")
	c.Check(err, jc.Satisfies, params.IsCodeTryAgain)
	// Let one request through, we should see that it succeeds without
	// error, and then be able to start a new request, but it will block
	delayChan <- struct{}{}
	select {
	case err := <-errResults:
		c.Check(err, gc.IsNil)
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
	// proceed, but the issued another one, so there should be
	// LoginRateLimit logins pending.
	for i := 0; i < apiserver.LoginRateLimit; i++ {
		delayChan <- struct{}{}
	}
	wg.Wait()
	close(errResults)
	for err := range errResults {
		c.Check(err, gc.IsNil)
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
	userInfo.Tag = names.NewUserTag("admin")
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
		c.Check(err, gc.IsNil)
	}
	c.Check(machineCount, gc.Equals, apiserver.LoginRateLimit)
	userCount := 0
	for err := range userResults {
		userCount += 1
		c.Check(err, gc.IsNil)
	}
	c.Check(userCount, gc.Equals, apiserver.LoginRateLimit+1)
}

func (s *loginSuite) TestUsersAreNotRateLimited(c *gc.C) {
	info, cleanup := s.setupServer(c)
	info.Tag = names.NewUserTag("admin")
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
		c.Check(err, gc.IsNil)
	}
}

func (s *loginSuite) TestLoginReportsEnvironTag(c *gc.C) {
	info, cleanup := s.setupServer(c)
	defer cleanup()
	// If we call api.Open without giving a username and password, then it
	// won't call Login, so we can call it ourselves.
	// We Login without passing an EnvironTag, to show that it still lets
	// us in, and that we can find out the real EnvironTag from the
	// response.
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer st.Close()
	var result params.LoginResult
	creds := &params.Creds{
		AuthTag:  "user-admin",
		Password: "dummy-secret",
	}
	err = st.Call("Admin", "", "Login", creds, &result)
	c.Assert(err, gc.IsNil)
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	c.Assert(result.EnvironTag, gc.Equals, env.Tag().String())
}

func (s *loginSuite) TestLoginValidationSuccess(c *gc.C) {
	validator := func(params.Creds) error {
		return nil
	}
	checker := func(c *gc.C, loginErr error, st *api.State) {
		c.Assert(loginErr, gc.IsNil)

		// Ensure an API call that would be restricted during
		// upgrades works after a normal login.
		err := st.Call("Client", "", "DestroyEnvironment", nil, nil)
		c.Assert(err, gc.IsNil)
	}
	s.checkLoginWithValidator(c, validator, checker)
}

func (s *loginSuite) TestLoginValidationFail(c *gc.C) {
	validator := func(params.Creds) error {
		return errors.New("Login not allowed")
	}
	checker := func(c *gc.C, loginErr error, _ *api.State) {
		c.Assert(loginErr, gc.ErrorMatches, "Login not allowed")
	}
	s.checkLoginWithValidator(c, validator, checker)
}

func (s *loginSuite) TestLoginValidationDuringUpgrade(c *gc.C) {
	validator := func(params.Creds) error {
		return apiserver.UpgradeInProgressError
	}
	checker := func(c *gc.C, loginErr error, st *api.State) {
		c.Assert(loginErr, gc.IsNil)

		var statusResult api.Status
		err := st.Call("Client", "", "FullStatus", params.StatusParams{}, &statusResult)
		c.Assert(err, gc.IsNil)

		err = st.Call("Client", "", "DestroyEnvironment", nil, nil)
		c.Assert(err, gc.ErrorMatches, "upgrade in progress - Juju functionality is limited")
	}
	s.checkLoginWithValidator(c, validator, checker)
}

type validationChecker func(c *gc.C, err error, st *api.State)

func (s *loginSuite) checkLoginWithValidator(c *gc.C, validator apiserver.LoginValidator, checker validationChecker) {
	info, cleanup := s.setupServerWithValidator(c, validator)
	defer cleanup()

	info.Tag = nil
	info.Password = ""

	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer st.Close()

	// Ensure not already logged in.
	_, err = st.Machiner().Machine(names.NewMachineTag("0"))
	c.Assert(err, gc.ErrorMatches, `unknown object type "Machiner"`)

	// Since these are user login tests, the nonce is empty.
	err = st.Login("user-admin", "dummy-secret", "")

	checker(c, err, st)
}

func (s *loginSuite) setupServerWithValidator(c *gc.C, validator apiserver.LoginValidator) (*api.Info, func()) {
	srv, err := apiserver.NewServer(
		s.State,
		apiserver.ServerConfig{
			Port:      0,
			Cert:      []byte(coretesting.ServerCert),
			Key:       []byte(coretesting.ServerKey),
			Validator: validator,
		},
	)
	c.Assert(err, gc.IsNil)
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	info := &api.Info{
		Tag:        nil,
		Password:   "",
		EnvironTag: env.Tag(),
		Addrs:      []string{srv.Addr()},
		CACert:     coretesting.CACert,
	}
	return info, func() {
		err := srv.Stop()
		c.Assert(err, gc.IsNil)
	}
}

func (s *loginSuite) TestLoginReportsAvailableFacadeVersions(c *gc.C) {
	info, cleanup := s.setupServer(c)
	defer cleanup()
	st, err := api.Open(info, fastDialOpts)
	c.Assert(err, gc.IsNil)
	defer st.Close()
	var result params.LoginResult
	creds := &params.Creds{
		AuthTag:  "user-admin",
		Password: "dummy-secret",
	}
	err = st.Call("Admin", "", "Login", creds, &result)
	c.Assert(err, gc.IsNil)
	c.Check(result.Facades, gc.Not(gc.HasLen), 0)
	// as a sanity check, ensure that we have Client v0
	asMap := make(map[string][]int, len(result.Facades))
	for _, facade := range result.Facades {
		asMap[facade.Name] = facade.Versions
	}
	clientVersions := asMap["Client"]
	c.Assert(len(clientVersions), jc.GreaterThan, 0)
	c.Check(clientVersions[0], gc.Equals, 0)
}
