// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type NewAPIConnSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&NewAPIConnSuite{})

func (cs *NewAPIConnSuite) SetUpTest(c *gc.C) {
	cs.LoggingSuite.SetUpTest(c)
	cs.ToolsFixture.SetUpTest(c)
}

func (cs *NewAPIConnSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.ToolsFixture.TearDownTest(c)
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewAPIConnSuite) TestNewConn(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	conn, err := juju.NewAPIConn(env, api.DefaultDialOpts())
	c.Assert(err, gc.IsNil)

	c.Assert(conn.Environ, gc.Equals, env)
	c.Assert(conn.State, gc.NotNil)

	c.Assert(conn.Close(), gc.IsNil)
}

type NewAPIClientSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&NewAPIClientSuite{})

func (cs *NewAPIClientSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewAPIClientSuite) TestNameDefault(c *gc.C) {
	defer coretesting.MakeMultipleEnvHome(c).Restore()
	// The connection logic should not delay the config connection
	// at all when there is no environment info available.
	// Make sure of that by providing a suitably long delay
	// and checking that the connection happens within that
	// time.
	defer testbase.PatchValue(juju.ProviderConnectDelay, coretesting.LongWait).Restore()
	bootstrapEnv(c, coretesting.SampleEnvName)

	startTime := time.Now()
	apiclient, err := juju.NewAPIClientFromName("")
	c.Assert(err, gc.IsNil)
	defer apiclient.Close()
	c.Assert(time.Since(startTime), jc.LessThan, coretesting.LongWait)

	// We should get the default sample environment if we ask for ""
	assertEnvironmentName(c, apiclient, coretesting.SampleEnvName)
}

func (*NewAPIClientSuite) TestNameNotDefault(c *gc.C) {
	defer coretesting.MakeMultipleEnvHome(c).Restore()
	// The default environment is "erewhemos", make sure we get the other one.
	const envName = "erewhemos-2"
	bootstrapEnv(c, envName)
	apiclient, err := juju.NewAPIClientFromName(envName)
	c.Assert(err, gc.IsNil)
	defer apiclient.Close()
	assertEnvironmentName(c, apiclient, envName)
}

func (*NewAPIClientSuite) TestWithInfoOnly(c *gc.C) {
	defer coretesting.MakeEmptyFakeHome(c).Restore()
	creds := configstore.APICredentials{
		User:     "foo",
		Password: "foopass",
	}
	endpoint := configstore.APIEndpoint{
		Addresses: []string{"foo.com"},
		CACert:    "certificated",
	}
	store := newConfigStore("noconfig", &environInfo{
		creds:    creds,
		endpoint: endpoint,
	})

	called := 0
	expectState := new(api.State)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (*api.State, error) {
		c.Check(apiInfo.Tag, gc.Equals, "user-foo")
		c.Check(string(apiInfo.CACert), gc.Equals, "certificated")
		c.Check(apiInfo.Tag, gc.Equals, "user-foo")
		c.Check(apiInfo.Password, gc.Equals, "foopass")
		c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
		called++
		return expectState, nil
	}
	defer testbase.PatchValue(juju.APIOpen, apiOpen).Restore()
	st, err := juju.NewAPIFromName("noconfig", store)
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
}

func (*NewAPIClientSuite) TestWithInfoError(c *gc.C) {
	defer coretesting.MakeEmptyFakeHome(c).Restore()
	expectErr := fmt.Errorf("an error")
	store := newConfigStore("noconfig", &environInfo{
		err: expectErr,
	})
	defer testbase.PatchValue(juju.APIOpen, panicAPIOpen).Restore()
	client, err := juju.NewAPIFromName("noconfig", store)
	c.Assert(err, gc.Equals, expectErr)
	c.Assert(client, gc.IsNil)
}

func panicAPIOpen(apiInfo *api.Info, opts api.DialOpts) (*api.State, error) {
	panic("api.Open called unexpectedly")
}

func (*NewAPIClientSuite) TestWithInfoNoAddresses(c *gc.C) {
	defer coretesting.MakeEmptyFakeHome(c).Restore()
	endpoint := configstore.APIEndpoint{
		Addresses: []string{},
		CACert:    "certificated",
	}
	store := newConfigStore("noconfig", &environInfo{
		endpoint: endpoint,
	})
	defer testbase.PatchValue(juju.APIOpen, panicAPIOpen).Restore()

	st, err := juju.NewAPIFromName("noconfig", store)
	c.Assert(err, gc.ErrorMatches, `environment "noconfig" not found`)
	c.Assert(st, gc.IsNil)
}

func (*NewAPIClientSuite) TestWithInfoAPIOpenError(c *gc.C) {
	defer coretesting.MakeEmptyFakeHome(c).Restore()
	endpoint := configstore.APIEndpoint{
		Addresses: []string{"foo.com"},
	}
	store := newConfigStore("noconfig", &environInfo{
		endpoint: endpoint,
	})

	expectErr := fmt.Errorf("an error")
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (*api.State, error) {
		return nil, expectErr
	}
	defer testbase.PatchValue(juju.APIOpen, apiOpen).Restore()
	st, err := juju.NewAPIFromName("noconfig", store)
	c.Assert(err, gc.Equals, expectErr)
	c.Assert(st, gc.IsNil)
}

func (*NewAPIClientSuite) TestWithSlowInfoConnect(c *gc.C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, coretesting.SampleEnvName)
	endpoint := configstore.APIEndpoint{
		Addresses: []string{"infoapi.com"},
	}
	store := newConfigStore(coretesting.SampleEnvName, &environInfo{
		endpoint: endpoint,
	})

	infoOpenedState := new(api.State)
	infoEndpointOpened := make(chan struct{})
	cfgOpenedState := new(api.State)
	// On a sample run with no delay, the logic took 45ms to run, so
	// we make the delay slightly more than that, so that if the
	// logic doesn't delay at all, the test will fail reasonably consistently.
	defer testbase.PatchValue(juju.ProviderConnectDelay, 50*time.Millisecond).Restore()
	apiOpen := func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		if info.Addrs[0] == "infoapi.com" {
			infoEndpointOpened <- struct{}{}
			return infoOpenedState, nil
		}
		return cfgOpenedState, nil
	}
	defer testbase.PatchValue(juju.APIOpen, apiOpen).Restore()

	stateClosed, restoreAPIClose := setAPIClosed()
	defer restoreAPIClose.Restore()

	startTime := time.Now()
	st, err := juju.NewAPIFromName(coretesting.SampleEnvName, store)
	c.Assert(err, gc.IsNil)
	// The connection logic should wait for some time before opening
	// the API from the configuration.
	c.Assert(time.Since(startTime), jc.GreaterThan, *juju.ProviderConnectDelay)
	c.Assert(st, gc.Equals, cfgOpenedState)

	select {
	case <-infoEndpointOpened:
	case <-time.After(coretesting.LongWait):
		c.Errorf("api never opened via info")
	}

	// Check that the ignored state was closed.
	select {
	case st := <-stateClosed:
		c.Assert(st, gc.Equals, infoOpenedState)
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for state to be closed")
	}
}

func (*NewAPIClientSuite) TestWithSlowConfigConnect(c *gc.C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, coretesting.SampleEnvName)
	endpoint := configstore.APIEndpoint{
		Addresses: []string{"infoapi.com"},
	}
	store := newConfigStore(coretesting.SampleEnvName, &environInfo{
		endpoint: endpoint,
	})

	infoOpenedState := new(api.State)
	infoEndpointOpened := make(chan struct{})
	cfgOpenedState := new(api.State)
	cfgEndpointOpened := make(chan struct{})

	defer testbase.PatchValue(juju.ProviderConnectDelay, 0*time.Second).Restore()
	apiOpen := func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		if info.Addrs[0] == "infoapi.com" {
			infoEndpointOpened <- struct{}{}
			<-infoEndpointOpened
			return infoOpenedState, nil
		}
		cfgEndpointOpened <- struct{}{}
		<-cfgEndpointOpened
		return cfgOpenedState, nil
	}
	defer testbase.PatchValue(juju.APIOpen, apiOpen).Restore()

	stateClosed, restoreAPIClose := setAPIClosed()
	defer restoreAPIClose.Restore()

	done := make(chan struct{})
	go func() {
		st, err := juju.NewAPIFromName(coretesting.SampleEnvName, store)
		c.Check(err, gc.IsNil)
		c.Check(st, gc.Equals, infoOpenedState)
		close(done)
	}()

	// Check that we're trying to connect to both endpoints:
	select {
	case <-infoEndpointOpened:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("api never opened via info")
	}
	select {
	case <-cfgEndpointOpened:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("api never opened via config")
	}
	// Let the info endpoint open go ahead and
	// check that the NewAPIFromName call returns.
	infoEndpointOpened <- struct{}{}
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out opening API")
	}

	// Let the config endpoint open go ahead and
	// check that its state is closed.
	cfgEndpointOpened <- struct{}{}
	select {
	case st := <-stateClosed:
		c.Assert(st, gc.Equals, cfgOpenedState)
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for state to be closed")
	}
}

func (*NewAPIClientSuite) TestBothErrror(c *gc.C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, coretesting.SampleEnvName)
	endpoint := configstore.APIEndpoint{
		Addresses: []string{"infoapi.com"},
	}
	store := newConfigStore(coretesting.SampleEnvName, &environInfo{
		endpoint: endpoint,
	})

	defer testbase.PatchValue(juju.ProviderConnectDelay, 0*time.Second).Restore()
	apiOpen := func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		if info.Addrs[0] == "infoapi.com" {
			return nil, fmt.Errorf("info connect failed")
		}
		return nil, fmt.Errorf("config connect failed")
	}
	defer testbase.PatchValue(juju.APIOpen, apiOpen).Restore()
	st, err := juju.NewAPIFromName(coretesting.SampleEnvName, store)
	c.Check(err, gc.ErrorMatches, "config connect failed")
	c.Check(st, gc.IsNil)
}

// TODO(jam): 2013-08-27 This should move somewhere in api.*
func (*NewAPIClientSuite) TestMultipleCloseOk(c *gc.C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, "")
	client, _ := juju.NewAPIClientFromName("")
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
}

func assertEnvironmentName(c *gc.C, client *api.Client, expectName string) {
	envInfo, err := client.EnvironmentInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(envInfo.Name, gc.Equals, expectName)
}

func setAPIClosed() (<-chan *api.State, testbase.Restorer) {
	stateClosed := make(chan *api.State)
	apiClose := func(st *api.State) error {
		stateClosed <- st
		return nil
	}
	return stateClosed, testbase.PatchValue(juju.APIClose, apiClose)
}

func newConfigStore(envName string, info *environInfo) configstore.Storage {
	return &configStorage{
		envs: map[string]*environInfo{
			envName: info,
		},
	}
}

type environInfo struct {
	configstore.EnvironInfo // panic on methods we don't care about
	creds                   configstore.APICredentials
	endpoint                configstore.APIEndpoint
	err                     error
}

type configStorage struct {
	configstore.Storage // panic on methods we don't care about
	envs                map[string]*environInfo
}

func (store *configStorage) ReadInfo(envName string) (configstore.EnvironInfo, error) {
	info := store.envs[envName]
	if info == nil {
		return nil, errors.NotFoundf("info on environment %q", envName)
	}
	if info.err != nil {
		return nil, info.err
	}
	return info, nil
}

func (info *environInfo) APICredentials() configstore.APICredentials {
	return info.creds
}

func (info *environInfo) APIEndpoint() configstore.APIEndpoint {
	return info.endpoint
}
