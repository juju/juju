// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
)

type NewAPIConnSuite struct {
	coretesting.FakeJujuHomeSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&NewAPIConnSuite{})

func (cs *NewAPIConnSuite) SetUpTest(c *gc.C) {
	cs.FakeJujuHomeSuite.SetUpTest(c)
	cs.ToolsFixture.SetUpTest(c)
}

func (cs *NewAPIConnSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.ToolsFixture.TearDownTest(c)
	cs.FakeJujuHomeSuite.TearDownTest(c)
}

func (*NewAPIConnSuite) TestNewConn(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	ctx := coretesting.Context(c)
	env, err := environs.Prepare(cfg, ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)

	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(ctx, env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	cfg = env.Config()
	cfg, err = cfg.Apply(map[string]interface{}{
		"secret": "fnord",
	})
	c.Assert(err, gc.IsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, gc.IsNil)

	conn, err := juju.NewAPIConn(env, api.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	c.Assert(conn.Environ, gc.Equals, env)
	c.Assert(conn.State, gc.NotNil)

	// the secrets will not be updated, as they already exist
	attrs, err := conn.State.Client().EnvironmentGet()
	c.Assert(attrs["secret"], gc.Equals, "pork")

	c.Assert(conn.Close(), gc.IsNil)
}

type NewAPIClientSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&NewAPIClientSuite{})

func (cs *NewAPIClientSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *NewAPIClientSuite) TestNameDefault(c *gc.C) {
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	// The connection logic should not delay the config connection
	// at all when there is no environment info available.
	// Make sure of that by providing a suitably long delay
	// and checking that the connection happens within that
	// time.
	s.PatchValue(juju.ProviderConnectDelay, coretesting.LongWait)
	bootstrapEnv(c, coretesting.SampleEnvName, defaultConfigStore(c))

	startTime := time.Now()
	apiclient, err := juju.NewAPIClientFromName("")
	c.Assert(err, gc.IsNil)
	defer apiclient.Close()
	c.Assert(time.Since(startTime), jc.LessThan, coretesting.LongWait)

	// We should get the default sample environment if we ask for ""
	assertEnvironmentName(c, apiclient, coretesting.SampleEnvName)
}

func (*NewAPIClientSuite) TestNameNotDefault(c *gc.C) {
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	envName := coretesting.SampleCertName + "-2"
	bootstrapEnv(c, envName, defaultConfigStore(c))
	apiclient, err := juju.NewAPIClientFromName(envName)
	c.Assert(err, gc.IsNil)
	defer apiclient.Close()
	assertEnvironmentName(c, apiclient, envName)
}

func (s *NewAPIClientSuite) TestWithInfoOnly(c *gc.C) {
	store := newConfigStore("noconfig", dummyStoreInfo)

	called := 0
	expectState := &mockAPIState{
		apiHostPorts: [][]instance.HostPort{
			instance.AddressesWithPort([]instance.Address{instance.NewAddress("0.1.2.3", instance.NetworkUnknown)}, 1234),
		},
	}
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (juju.APIState, error) {
		c.Check(apiInfo.Tag, gc.Equals, "user-foo")
		c.Check(string(apiInfo.CACert), gc.Equals, "certificated")
		c.Check(apiInfo.Password, gc.Equals, "foopass")
		c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
		called++
		return expectState, nil
	}

	// Give NewAPIFromStore a store interface that can report when the
	// config was written to, to check if the cache is updated.
	mockStore := &storageWithWriteNotify{store: store}
	st, err := juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err := store.ReadInfo("noconfig")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APIEndpoint().Addresses, gc.DeepEquals, []string{"0.1.2.3:1234"})
	mockStore.written = false

	// If APIHostPorts haven't changed, then the store won't be updated.
	st, err = juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 2)
	c.Assert(mockStore.written, jc.IsFalse)
}

func (s *NewAPIClientSuite) TestWithConfigAndNoInfo(c *gc.C) {
	coretesting.MakeSampleJujuHome(c)

	store := newConfigStore(coretesting.SampleEnvName, &environInfo{
		bootstrapConfig: map[string]interface{}{
			"type":                      "dummy",
			"name":                      "myenv",
			"state-server":              true,
			"authorized-keys":           "i-am-a-key",
			"default-series":            config.LatestLtsSeries(),
			"firewall-mode":             config.FwInstance,
			"development":               false,
			"ssl-hostname-verification": true,
			"admin-secret":              "adminpass",
		},
	})
	bootstrapEnv(c, coretesting.SampleEnvName, store)

	// Verify the cache is empty.
	info, err := store.ReadInfo("myenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.NotNil)
	c.Assert(info.APIEndpoint(), jc.DeepEquals, configstore.APIEndpoint{})
	c.Assert(info.APICredentials(), jc.DeepEquals, configstore.APICredentials{})

	called := 0
	expectState := &mockAPIState{}
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (juju.APIState, error) {
		c.Check(apiInfo.Tag, gc.Equals, "user-admin")
		c.Check(string(apiInfo.CACert), gc.Not(gc.Equals), "")
		c.Check(apiInfo.Password, gc.Equals, "adminpass")
		c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
		called++
		return expectState, nil
	}
	st, err := juju.NewAPIFromStore("myenv", store, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)

	// Make sure the cache is updated.
	info, err = store.ReadInfo("myenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.NotNil)
	ep := info.APIEndpoint()
	c.Assert(ep.Addresses, gc.HasLen, 1)
	c.Check(ep.Addresses[0], gc.Matches, `127\.0\.0\.1:\d+`)
	c.Check(ep.CACert, gc.Not(gc.Equals), "")
	creds := info.APICredentials()
	c.Check(creds.User, gc.Equals, "admin")
	c.Check(creds.Password, gc.Equals, "adminpass")
}

func (s *NewAPIClientSuite) TestWithInfoError(c *gc.C) {
	expectErr := fmt.Errorf("an error")
	store := newConfigStoreWithError(expectErr)
	client, err := juju.NewAPIFromStore("noconfig", store, panicAPIOpen)
	c.Assert(err, gc.Equals, expectErr)
	c.Assert(client, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithInfoNoAddresses(c *gc.C) {
	store := newConfigStore("noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			Addresses: []string{},
			CACert:    "certificated",
		},
	})
	st, err := juju.NewAPIFromStore("noconfig", store, panicAPIOpen)
	c.Assert(err, gc.ErrorMatches, `environment "noconfig" not found`)
	c.Assert(st, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithInfoAPIOpenError(c *gc.C) {
	store := newConfigStore("noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			Addresses: []string{"foo.invalid"},
		},
	})

	expectErr := fmt.Errorf("an error")
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (juju.APIState, error) {
		return nil, expectErr
	}
	st, err := juju.NewAPIFromStore("noconfig", store, apiOpen)
	c.Assert(err, gc.Equals, expectErr)
	c.Assert(st, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithSlowInfoConnect(c *gc.C) {
	coretesting.MakeSampleJujuHome(c)
	store := configstore.NewMem()
	bootstrapEnv(c, coretesting.SampleEnvName, store)
	setEndpointAddress(c, store, coretesting.SampleEnvName, "infoapi.invalid")

	infoOpenedState := &mockAPIState{}
	infoEndpointOpened := make(chan struct{})
	cfgOpenedState := &mockAPIState{}
	// On a sample run with no delay, the logic took 45ms to run, so
	// we make the delay slightly more than that, so that if the
	// logic doesn't delay at all, the test will fail reasonably consistently.
	s.PatchValue(juju.ProviderConnectDelay, 50*time.Millisecond)
	apiOpen := func(info *api.Info, opts api.DialOpts) (juju.APIState, error) {
		if info.Addrs[0] == "infoapi.invalid" {
			infoEndpointOpened <- struct{}{}
			return infoOpenedState, nil
		}
		return cfgOpenedState, nil
	}

	stateClosed := make(chan juju.APIState)
	infoOpenedState.close = func(st juju.APIState) error {
		stateClosed <- st
		return nil
	}
	cfgOpenedState.close = infoOpenedState.close

	startTime := time.Now()
	st, err := juju.NewAPIFromStore(coretesting.SampleEnvName, store, apiOpen)
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

type badBootstrapInfo struct {
	configstore.EnvironInfo
}

// BootstrapConfig is returned as a map with real content, but the content
// isn't actually valid configuration, causing config.New to fail
func (m *badBootstrapInfo) BootstrapConfig() map[string]interface{} {
	return map[string]interface{}{"something": "else"}
}

func (s *NewAPIClientSuite) TestBadConfigDoesntPanic(c *gc.C) {
	badInfo := &badBootstrapInfo{}
	cfg, err := juju.GetConfig(badInfo, nil, "test")
	// The specific error we get depends on what key is invalid, which is a
	// bit spurious, but what we care about is that we didn't get a panic,
	// but instead got an error
	c.Assert(err, gc.ErrorMatches, ".*expected.*got nothing")
	c.Assert(cfg, gc.IsNil)
}

func setEndpointAddress(c *gc.C, store configstore.Storage, envName string, addr string) {
	// Populate the environment's info with an endpoint
	// with a known address.
	info, err := store.ReadInfo(coretesting.SampleEnvName)
	c.Assert(err, gc.IsNil)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: []string{addr},
		CACert:    "certificated",
	})
	err = info.Write()
	c.Assert(err, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithSlowConfigConnect(c *gc.C) {
	coretesting.MakeSampleJujuHome(c)

	store := configstore.NewMem()
	bootstrapEnv(c, coretesting.SampleEnvName, store)
	setEndpointAddress(c, store, coretesting.SampleEnvName, "infoapi.invalid")

	infoOpenedState := &mockAPIState{}
	infoEndpointOpened := make(chan struct{})
	cfgOpenedState := &mockAPIState{}
	cfgEndpointOpened := make(chan struct{})

	s.PatchValue(juju.ProviderConnectDelay, 0*time.Second)
	apiOpen := func(info *api.Info, opts api.DialOpts) (juju.APIState, error) {
		if info.Addrs[0] == "infoapi.invalid" {
			infoEndpointOpened <- struct{}{}
			<-infoEndpointOpened
			return infoOpenedState, nil
		}
		cfgEndpointOpened <- struct{}{}
		<-cfgEndpointOpened
		return cfgOpenedState, nil
	}

	stateClosed := make(chan juju.APIState)
	infoOpenedState.close = func(st juju.APIState) error {
		stateClosed <- st
		return nil
	}
	cfgOpenedState.close = infoOpenedState.close

	done := make(chan struct{})
	go func() {
		st, err := juju.NewAPIFromStore(coretesting.SampleEnvName, store, apiOpen)
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
	// check that the NewAPIFromStore call returns.
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

func (s *NewAPIClientSuite) TestBothError(c *gc.C) {
	coretesting.MakeSampleJujuHome(c)
	store := configstore.NewMem()
	bootstrapEnv(c, coretesting.SampleEnvName, store)
	setEndpointAddress(c, store, coretesting.SampleEnvName, "infoapi.invalid")

	s.PatchValue(juju.ProviderConnectDelay, 0*time.Second)
	apiOpen := func(info *api.Info, opts api.DialOpts) (juju.APIState, error) {
		if info.Addrs[0] == "infoapi.invalid" {
			return nil, fmt.Errorf("info connect failed")
		}
		return nil, fmt.Errorf("config connect failed")
	}
	st, err := juju.NewAPIFromStore(coretesting.SampleEnvName, store, apiOpen)
	c.Check(err, gc.ErrorMatches, "config connect failed")
	c.Check(st, gc.IsNil)
}

func defaultConfigStore(c *gc.C) configstore.Storage {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	return store
}

// TODO(jam): 2013-08-27 This should move somewhere in api.*
func (s *NewAPIClientSuite) TestMultipleCloseOk(c *gc.C) {
	coretesting.MakeSampleJujuHome(c)
	bootstrapEnv(c, "", defaultConfigStore(c))
	client, _ := juju.NewAPIClientFromName("")
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
	c.Assert(client.Close(), gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithBootstrapConfigAndNoEnvironmentsFile(c *gc.C) {
	coretesting.MakeSampleJujuHome(c)
	store := configstore.NewMem()
	bootstrapEnv(c, coretesting.SampleEnvName, store)
	info, err := store.ReadInfo(coretesting.SampleEnvName)
	c.Assert(err, gc.IsNil)
	c.Assert(info.BootstrapConfig(), gc.NotNil)
	c.Assert(info.APIEndpoint().Addresses, gc.HasLen, 0)

	err = os.Remove(osenv.JujuHomePath("environments.yaml"))
	c.Assert(err, gc.IsNil)

	apiOpen := func(*api.Info, api.DialOpts) (juju.APIState, error) {
		return &mockAPIState{}, nil
	}
	st, err := juju.NewAPIFromStore(coretesting.SampleEnvName, store, apiOpen)
	c.Check(err, gc.IsNil)
	st.Close()
}

func (*NewAPIClientSuite) TestWithBootstrapConfigTakesPrecedence(c *gc.C) {
	// We want to make sure that the code is using the bootstrap
	// config rather than information from environments.yaml,
	// even when there is an entry in environments.yaml
	// We can do that by changing the info bootstrap config
	// so it has a different environment name.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)

	store := configstore.NewMem()
	bootstrapEnv(c, coretesting.SampleEnvName, store)
	info, err := store.ReadInfo(coretesting.SampleEnvName)
	c.Assert(err, gc.IsNil)

	envName2 := coretesting.SampleCertName + "-2"
	info2, err := store.CreateInfo(envName2)
	c.Assert(err, gc.IsNil)
	info2.SetBootstrapConfig(info.BootstrapConfig())
	err = info2.Write()
	c.Assert(err, gc.IsNil)

	// Now we have info for envName2 which will actually
	// cause a connection to the originally bootstrapped
	// state.
	apiOpen := func(*api.Info, api.DialOpts) (juju.APIState, error) {
		return &mockAPIState{}, nil
	}
	st, err := juju.NewAPIFromStore(envName2, store, apiOpen)
	c.Check(err, gc.IsNil)
	st.Close()

	// Sanity check that connecting to the envName2
	// but with no info fails.
	// Currently this panics with an "environment not prepared" error.
	// Disable for now until an upcoming branch fixes it.
	//	err = info2.Destroy()
	//	c.Assert(err, gc.IsNil)
	//	st, err = juju.NewAPIFromStore(envName2, store)
	//	if err == nil {
	//		st.Close()
	//	}
	//	c.Assert(err, gc.ErrorMatches, "fooobie")
}

func assertEnvironmentName(c *gc.C, client *api.Client, expectName string) {
	envInfo, err := client.EnvironmentInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(envInfo.Name, gc.Equals, expectName)
}

// newConfigStoreWithError that will return the given
// error from ReadInfo.
func newConfigStoreWithError(err error) configstore.Storage {
	return &errorConfigStorage{
		Storage: configstore.NewMem(),
		err:     err,
	}
}

type errorConfigStorage struct {
	configstore.Storage
	err error
}

func (store *errorConfigStorage) ReadInfo(envName string) (configstore.EnvironInfo, error) {
	return nil, store.err
}

type environInfo struct {
	creds           configstore.APICredentials
	endpoint        configstore.APIEndpoint
	bootstrapConfig map[string]interface{}
}

// newConfigStore returns a storage that contains information
// for the environment name.
func newConfigStore(envName string, info *environInfo) configstore.Storage {
	store := configstore.NewMem()
	newInfo, err := store.CreateInfo(envName)
	if err != nil {
		panic(err)
	}
	newInfo.SetAPICredentials(info.creds)
	newInfo.SetAPIEndpoint(info.endpoint)
	newInfo.SetBootstrapConfig(info.bootstrapConfig)
	err = newInfo.Write()
	if err != nil {
		panic(err)
	}
	return store
}

type storageWithWriteNotify struct {
	written bool
	store   configstore.Storage
}

func (*storageWithWriteNotify) CreateInfo(envName string) (configstore.EnvironInfo, error) {
	panic("CreateInfo not implemented")
}

func (s *storageWithWriteNotify) ReadInfo(envName string) (configstore.EnvironInfo, error) {
	info, err := s.store.ReadInfo(envName)
	if err != nil {
		return nil, err
	}
	return &infoWithWriteNotify{
		written:     &s.written,
		EnvironInfo: info,
	}, nil
}

type infoWithWriteNotify struct {
	configstore.EnvironInfo
	written *bool
}

func (info *infoWithWriteNotify) Write() error {
	*info.written = true
	return info.EnvironInfo.Write()
}

type APIEndpointForEnvSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&APIEndpointForEnvSuite{})

var dummyStoreInfo = &environInfo{
	creds: configstore.APICredentials{
		User:     "foo",
		Password: "foopass",
	},
	endpoint: configstore.APIEndpoint{
		Addresses: []string{"foo.invalid"},
		CACert:    "certificated",
	},
}

func (s *APIEndpointForEnvSuite) TestAPIEndpointInStoreCached(c *gc.C) {
	store := newConfigStore("env-name", dummyStoreInfo)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (juju.APIState, error) {
		return nil, nil
	}
	endpoint, err := juju.APIEndpointInStore("env-name", false, store, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Check(endpoint, gc.DeepEquals, dummyStoreInfo.endpoint)
}

func (s *APIEndpointForEnvSuite) TestAPIEndpointForEnvSuchName(c *gc.C) {
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	_, err := juju.APIEndpointForEnv("no-such-env", false)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `environment "no-such-env" not found`)
}

func (s *APIEndpointForEnvSuite) TestAPIEndpointNotCached(c *gc.C) {
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	ctx := coretesting.Context(c)
	env, err := environs.PrepareFromName("erewhemos", ctx, store)
	c.Assert(err, gc.IsNil)
	defer dummy.Reset()
	envtesting.UploadFakeTools(c, env.Storage())
	err = bootstrap.Bootstrap(ctx, env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	// Note: if we get Bootstrap to start caching the API endpoint
	// immediately, we'll still want to have this test for compatibility.
	// We can just write blank info instead of reading and checking it is empty.
	savedInfo, err := store.ReadInfo("erewhemos")
	c.Assert(err, gc.IsNil)
	// Ensure that the data isn't cached
	c.Check(savedInfo.APIEndpoint().Addresses, gc.HasLen, 0)

	called := 0
	expectState := &mockAPIState{
		apiHostPorts: [][]instance.HostPort{
			instance.AddressesWithPort([]instance.Address{instance.NewAddress("0.1.2.3", instance.NetworkUnknown)}, 1234),
		},
	}
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (juju.APIState, error) {
		c.Check(apiInfo.Tag, gc.Equals, "user-admin")
		c.Check(string(apiInfo.CACert), gc.Equals, coretesting.CACert)
		c.Check(apiInfo.Password, gc.Equals, coretesting.DefaultMongoPassword)
		c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
		called++
		return expectState, nil
	}
	endpoint, err := juju.APIEndpointInStore("erewhemos", false, store, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
	c.Check(endpoint.Addresses, gc.DeepEquals, []string{"0.1.2.3:1234"})
}

func (s *APIEndpointForEnvSuite) TestAPIEndpointRefresh(c *gc.C) {
	store := newConfigStore("env-name", dummyStoreInfo)
	called := 0
	expectState := &mockAPIState{
		apiHostPorts: [][]instance.HostPort{
			instance.AddressesWithPort([]instance.Address{instance.NewAddress("0.1.2.3", instance.NetworkUnknown)}, 1234),
		},
	}
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (juju.APIState, error) {
		c.Check(apiInfo.Tag, gc.Equals, "user-foo")
		c.Check(string(apiInfo.CACert), gc.Equals, "certificated")
		c.Check(apiInfo.Password, gc.Equals, "foopass")
		c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
		called++
		return expectState, nil
	}
	endpoint, err := juju.APIEndpointInStore("env-name", false, store, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Assert(called, gc.Equals, 0)
	c.Check(endpoint.Addresses, gc.DeepEquals, []string{"foo.invalid"})
	// However, if we ask to refresh them, we'll connect to the API and get
	// the freshest set
	endpoint, err = juju.APIEndpointInStore("env-name", true, store, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Check(called, gc.Equals, 1)
	// This refresh now gives us the values return by APIHostPorts
	c.Check(endpoint.Addresses, gc.DeepEquals, []string{"0.1.2.3:1234"})
}

func (s *APIEndpointForEnvSuite) TestAPIEndpointNotMachineLocal(c *gc.C) {
	store := newConfigStore("env-name", dummyStoreInfo)
	called := 0
	hostPorts := [][]instance.HostPort{
		instance.AddressesWithPort([]instance.Address{
			instance.NewAddress("1.0.0.1", instance.NetworkPublic),
			instance.NewAddress("192.0.0.1", instance.NetworkCloudLocal),
			instance.NewAddress("127.0.0.1", instance.NetworkMachineLocal),
			instance.NewAddress("localhost", instance.NetworkMachineLocal),
		}, 1234),
		instance.AddressesWithPort([]instance.Address{
			instance.NewAddress("1.0.0.2", instance.NetworkUnknown),
			instance.NewAddress("2002:0:0:0:0:0:100:2", instance.NetworkUnknown),
			instance.NewAddress("::1", instance.NetworkMachineLocal),
			instance.NewAddress("127.0.0.1", instance.NetworkMachineLocal),
			instance.NewAddress("localhost", instance.NetworkMachineLocal),
		}, 1235),
	}

	expectState := &mockAPIState{apiHostPorts: hostPorts}
	apiOpen := func(_ *api.Info, _ api.DialOpts) (juju.APIState, error) {
		called++
		return expectState, nil
	}
	endpoint, err := juju.APIEndpointInStore("env-name", true, store, apiOpen)
	c.Assert(err, gc.IsNil)
	c.Check(called, gc.Equals, 1)
	c.Check(endpoint.Addresses, gc.DeepEquals, []string{
		"1.0.0.1:1234",
		"192.0.0.1:1234",
		"1.0.0.2:1235",
	})
}
