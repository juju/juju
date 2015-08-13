// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type NewAPIStateSuite struct {
	coretesting.FakeJujuHomeSuite
	testing.MgoSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&NewAPIStateSuite{})

func (cs *NewAPIStateSuite) SetUpSuite(c *gc.C) {
	cs.FakeJujuHomeSuite.SetUpSuite(c)
	cs.MgoSuite.SetUpSuite(c)
}

func (cs *NewAPIStateSuite) TearDownSuite(c *gc.C) {
	cs.MgoSuite.TearDownSuite(c)
	cs.FakeJujuHomeSuite.TearDownSuite(c)
}

func (cs *NewAPIStateSuite) SetUpTest(c *gc.C) {
	cs.FakeJujuHomeSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
	cs.ToolsFixture.SetUpTest(c)
}

func (cs *NewAPIStateSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.ToolsFixture.TearDownTest(c)
	cs.MgoSuite.TearDownTest(c)
	cs.FakeJujuHomeSuite.TearDownTest(c)
}

func (cs *NewAPIStateSuite) TestNewAPIState(c *gc.C) {
	cs.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(c)
	env, err := environs.Prepare(cfg, ctx, configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)

	storageDir := c.MkDir()
	cs.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	cfg = env.Config()
	cfg, err = cfg.Apply(map[string]interface{}{
		"secret": "fnord",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	st, err := juju.NewAPIState(dummy.AdminUserTag(), env, api.DialOpts{})
	c.Assert(st, gc.NotNil)

	// the secrets will not be updated, as they already exist
	attrs, err := st.Client().EnvironmentGet()
	c.Assert(attrs["secret"], gc.Equals, "pork")

	c.Assert(st.Close(), gc.IsNil)
}

type NewAPIClientSuite struct {
	coretesting.FakeJujuHomeSuite
	testing.MgoSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&NewAPIClientSuite{})

func (cs *NewAPIClientSuite) SetUpSuite(c *gc.C) {
	cs.FakeJujuHomeSuite.SetUpSuite(c)
	cs.MgoSuite.SetUpSuite(c)
	// Since most tests use invalid testing API server addresses, we
	// need to mock this to avoid errors.
	cs.PatchValue(juju.ServerAddress, func(addr string) (network.HostPort, error) {
		host, strPort, err := net.SplitHostPort(addr)
		if err != nil {
			c.Logf("serverAddress %q invalid, ignoring error: %v", addr, err)
		}
		port, err := strconv.Atoi(strPort)
		if err != nil {
			c.Logf("serverAddress %q port, ignoring error: %v", addr, err)
			port = 0
		}
		return network.NewHostPorts(port, host)[0], nil
	})
}

func (cs *NewAPIClientSuite) TearDownSuite(c *gc.C) {
	cs.MgoSuite.TearDownSuite(c)
	cs.FakeJujuHomeSuite.TearDownSuite(c)
}

func (cs *NewAPIClientSuite) SetUpTest(c *gc.C) {
	cs.ToolsFixture.SetUpTest(c)
	cs.FakeJujuHomeSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
}

func (cs *NewAPIClientSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.ToolsFixture.TearDownTest(c)
	cs.MgoSuite.TearDownTest(c)
	cs.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *NewAPIClientSuite) bootstrapEnv(c *gc.C, envName string, store configstore.Storage) {
	if store == nil {
		store = configstore.NewMem()
	}
	ctx := envtesting.BootstrapContext(c)
	c.Logf("env name: %s", envName)
	env, err := environs.PrepareFromName(envName, ctx, store)
	c.Assert(err, jc.ErrorIsNil)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	info, err := store.ReadInfo(envName)
	c.Assert(err, jc.ErrorIsNil)
	creds := info.APICredentials()
	creds.User = dummy.AdminUserTag().Name()
	c.Logf("set creds: %#v", creds)
	info.SetAPICredentials(creds)
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("creds: %#v", info.APICredentials())
	info, err = store.ReadInfo(envName)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("read creds: %#v", info.APICredentials())
	c.Logf("store: %#v", store)
}

func (s *NewAPIClientSuite) TestNameDefault(c *gc.C) {
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	// The connection logic should not delay the config connection
	// at all when there is no environment info available.
	// Make sure of that by providing a suitably long delay
	// and checking that the connection happens within that
	// time.
	s.PatchValue(juju.ProviderConnectDelay, coretesting.LongWait)
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	s.bootstrapEnv(c, coretesting.SampleEnvName, defaultConfigStore(c))

	startTime := time.Now()
	apiclient, err := juju.NewAPIClientFromName("")
	c.Assert(err, jc.ErrorIsNil)
	defer apiclient.Close()
	c.Assert(time.Since(startTime), jc.LessThan, coretesting.LongWait)

	// We should get the default sample environment if we ask for ""
	assertEnvironmentName(c, apiclient, coretesting.SampleEnvName)
}

func (s *NewAPIClientSuite) TestNameNotDefault(c *gc.C) {
	envName := coretesting.SampleCertName + "-2"
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig, envName)
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	s.bootstrapEnv(c, envName, defaultConfigStore(c))
	apiclient, err := juju.NewAPIClientFromName(envName)
	c.Assert(err, jc.ErrorIsNil)
	defer apiclient.Close()
	assertEnvironmentName(c, apiclient, envName)
}

func (s *NewAPIClientSuite) TestWithInfoOnly(c *gc.C) {
	store := newConfigStore("noconfig", dummyStoreInfo)

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedEnvironTag)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.EnvironTag, gc.Equals, names.NewEnvironTag(fakeUUID))
		called++
		return expectState, nil
	}

	// Give NewAPIFromStore a store interface that can report when the
	// config was written to, to check if the cache is updated.
	mockStore := &storageWithWriteNotify{store: store}
	st, err := juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err := store.ReadInfo("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	ep := info.APIEndpoint()
	c.Check(ep.Addresses, jc.DeepEquals, []string{
		"0.1.2.3:1234", "[2001:db8::1]:1234",
	})
	c.Check(ep.EnvironUUID, gc.Equals, fakeUUID)
	mockStore.written = false

	// If APIHostPorts haven't changed, then the store won't be updated.
	st, err = juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 2)
	c.Assert(mockStore.written, jc.IsFalse)
}

func (s *NewAPIClientSuite) TestWithConfigAndNoInfo(c *gc.C) {
	c.Skip("not really possible now that there is no defined admin user")
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
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
	s.bootstrapEnv(c, coretesting.SampleEnvName, store)

	info, err := store.ReadInfo("myenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.NotNil)
	c.Logf("%#v", info.APICredentials())

	called := 0
	expectState := mockedAPIState(0)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		c.Check(apiInfo.Tag, gc.Equals, dummy.AdminUserTag())
		c.Check(string(apiInfo.CACert), gc.Not(gc.Equals), "")
		c.Check(apiInfo.Password, gc.Equals, "adminpass")
		// EnvironTag wasn't in regular Config
		c.Check(apiInfo.EnvironTag.Id(), gc.Equals, "")
		c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
		called++
		return expectState, nil
	}
	st, err := juju.NewAPIFromStore("myenv", store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)

	// Make sure the cache is updated.
	info, err = store.ReadInfo("myenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.NotNil)
	ep := info.APIEndpoint()
	c.Assert(ep.Addresses, gc.HasLen, 1)
	c.Check(ep.Addresses[0], gc.Matches, `localhost:\d+`)
	c.Check(ep.CACert, gc.Not(gc.Equals), "")
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

var noTagStoreInfo = &environInfo{
	creds: configstore.APICredentials{
		User:     "foo",
		Password: "foopass",
	},
	endpoint: configstore.APIEndpoint{
		Addresses: []string{"foo.invalid"},
		CACert:    "certificated",
	},
}

type mockedStateFlags int

const (
	noFlags          mockedStateFlags = 0x0000
	mockedHostPort   mockedStateFlags = 0x0001
	mockedEnvironTag mockedStateFlags = 0x0002
	mockedPreferIPv6 mockedStateFlags = 0x0004
)

func mockedAPIState(flags mockedStateFlags) *mockAPIState {
	hasHostPort := flags&mockedHostPort == mockedHostPort
	hasEnvironTag := flags&mockedEnvironTag == mockedEnvironTag
	preferIPv6 := flags&mockedPreferIPv6 == mockedPreferIPv6
	addr := ""

	apiHostPorts := [][]network.HostPort{}
	if hasHostPort {
		var apiAddrs []network.Address
		ipv4Address := network.NewAddress("0.1.2.3")
		ipv6Address := network.NewAddress("2001:db8::1")
		if preferIPv6 {
			addr = net.JoinHostPort(ipv6Address.Value, "1234")
			apiAddrs = append(apiAddrs, ipv6Address, ipv4Address)
		} else {
			addr = net.JoinHostPort(ipv4Address.Value, "1234")
			apiAddrs = append(apiAddrs, ipv4Address, ipv6Address)
		}
		apiHostPorts = [][]network.HostPort{
			network.AddressesWithPort(apiAddrs, 1234),
		}
	}
	environTag := ""
	if hasEnvironTag {
		environTag = "environment-df136476-12e9-11e4-8a70-b2227cce2b54"
	}
	return &mockAPIState{
		apiHostPorts: apiHostPorts,
		environTag:   environTag,
		addr:         addr,
	}
}

func checkCommonAPIInfoAttrs(c *gc.C, apiInfo *api.Info, opts api.DialOpts) {
	c.Check(apiInfo.Tag, gc.Equals, names.NewUserTag("foo"))
	c.Check(string(apiInfo.CACert), gc.Equals, "certificated")
	c.Check(apiInfo.Password, gc.Equals, "foopass")
	c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
}

func (s *NewAPIClientSuite) TestWithInfoNoEnvironTag(c *gc.C) {
	store := newConfigStore("noconfig", noTagStoreInfo)

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedEnvironTag)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.EnvironTag.Id(), gc.Equals, "")
		called++
		return expectState, nil
	}

	// Give NewAPIFromStore a store interface that can report when the
	// config was written to, to check if the cache is updated.
	mockStore := &storageWithWriteNotify{store: store}
	st, err := juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err := store.ReadInfo("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info.APIEndpoint().Addresses, jc.DeepEquals, []string{
		"0.1.2.3:1234", "[2001:db8::1]:1234",
	})
	c.Check(info.APIEndpoint().EnvironUUID, gc.Equals, fakeUUID)

	// Now simulate prefer-ipv6: true
	store = newConfigStore("noconfig", noTagStoreInfo)
	mockStore = &storageWithWriteNotify{store: store}
	s.PatchValue(juju.MaybePreferIPv6, func(_ configstore.EnvironInfo) bool {
		return true
	})
	expectState = mockedAPIState(mockedHostPort | mockedEnvironTag | mockedPreferIPv6)
	st, err = juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 2)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err = store.ReadInfo("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info.APIEndpoint().Addresses, jc.DeepEquals, []string{
		"[2001:db8::1]:1234", "0.1.2.3:1234",
	})
	c.Check(info.APIEndpoint().EnvironUUID, gc.Equals, fakeUUID)
}

func (s *NewAPIClientSuite) TestWithInfoNoAPIHostports(c *gc.C) {
	// The local cache doesn't have an EnvironTag, which the API does
	// return. However, the API doesn't have apiHostPorts, we don't want to
	// override the local cache with bad endpoints.
	store := newConfigStore("noconfig", noTagStoreInfo)

	called := 0
	expectState := mockedAPIState(mockedEnvironTag | mockedPreferIPv6)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.EnvironTag.Id(), gc.Equals, "")
		called++
		return expectState, nil
	}

	mockStore := &storageWithWriteNotify{store: store}
	st, err := juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err := store.ReadInfo("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	ep := info.APIEndpoint()
	// We should have cached the environ tag, but not disturbed the
	// Addresses
	c.Check(ep.Addresses, gc.HasLen, 1)
	c.Check(ep.Addresses[0], gc.Matches, `foo\.invalid`)
	c.Check(ep.EnvironUUID, gc.Equals, fakeUUID)
}

func (s *NewAPIClientSuite) TestNoEnvironTagDoesntOverwriteCached(c *gc.C) {
	store := newConfigStore("noconfig", dummyStoreInfo)
	called := 0
	// State returns a new set of APIHostPorts but not a new EnvironTag. We
	// shouldn't override the cached value with environ tag of "".
	expectState := mockedAPIState(mockedHostPort)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.EnvironTag, gc.Equals, names.NewEnvironTag(fakeUUID))
		called++
		return expectState, nil
	}

	mockStore := &storageWithWriteNotify{store: store}
	st, err := juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err := store.ReadInfo("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	ep := info.APIEndpoint()
	c.Check(ep.Addresses, gc.DeepEquals, []string{
		"0.1.2.3:1234", "[2001:db8::1]:1234",
	})
	c.Check(ep.EnvironUUID, gc.Equals, fakeUUID)

	// Now simulate prefer-ipv6: true
	s.PatchValue(juju.MaybePreferIPv6, func(_ configstore.EnvironInfo) bool {
		return true
	})
	expectState = mockedAPIState(mockedHostPort | mockedPreferIPv6)
	st, err = juju.NewAPIFromStore("noconfig", mockStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 2)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err = store.ReadInfo("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	ep = info.APIEndpoint()
	c.Check(ep.Addresses, gc.DeepEquals, []string{
		"[2001:db8::1]:1234", "0.1.2.3:1234",
	})
	c.Check(ep.EnvironUUID, gc.Equals, fakeUUID)
}

func (s *NewAPIClientSuite) TestWithInfoAPIOpenError(c *gc.C) {
	store := newConfigStore("noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			Addresses: []string{"foo.invalid"},
		},
	})

	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		return nil, errors.Errorf("an error")
	}
	st, err := juju.NewAPIFromStore("noconfig", store, apiOpen)
	// We expect to  get the isNotFound error as it is more important than the
	// infoConnectError "an error"
	c.Assert(err, gc.ErrorMatches, "environment \"noconfig\" not found")
	c.Assert(st, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithSlowInfoConnect(c *gc.C) {
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	coretesting.MakeSampleJujuHome(c)
	store := configstore.NewMem()
	s.bootstrapEnv(c, coretesting.SampleEnvName, store)
	setEndpointAddressAndHostname(c, store, coretesting.SampleEnvName, "0.1.2.3", "infoapi.invalid")

	infoOpenedState := mockedAPIState(noFlags)
	infoEndpointOpened := make(chan struct{})
	cfgOpenedState := mockedAPIState(noFlags)
	// On a sample run with no delay, the logic took 45ms to run, so
	// we make the delay slightly more than that, so that if the
	// logic doesn't delay at all, the test will fail reasonably consistently.
	s.PatchValue(juju.ProviderConnectDelay, 50*time.Millisecond)
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		if info.Addrs[0] == "0.1.2.3" {
			infoEndpointOpened <- struct{}{}
			return infoOpenedState, nil
		}
		return cfgOpenedState, nil
	}

	stateClosed := make(chan api.Connection)
	infoOpenedState.close = func(st api.Connection) error {
		stateClosed <- st
		return nil
	}
	cfgOpenedState.close = infoOpenedState.close

	startTime := time.Now()
	st, err := juju.NewAPIFromStore(coretesting.SampleEnvName, store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
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

func setEndpointAddressAndHostname(c *gc.C, store configstore.Storage, envName string, addr, host string) {
	// Populate the environment's info with an endpoint
	// with a known address and hostname.
	info, err := store.ReadInfo(coretesting.SampleEnvName)
	c.Assert(err, jc.ErrorIsNil)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: []string{addr},
		Hostnames: []string{host},
		CACert:    "certificated",
	})
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NewAPIClientSuite) TestWithSlowConfigConnect(c *gc.C) {
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	coretesting.MakeSampleJujuHome(c)

	store := configstore.NewMem()
	s.bootstrapEnv(c, coretesting.SampleEnvName, store)
	setEndpointAddressAndHostname(c, store, coretesting.SampleEnvName, "0.1.2.3", "infoapi.invalid")

	infoOpenedState := mockedAPIState(noFlags)
	infoEndpointOpened := make(chan struct{})
	cfgOpenedState := mockedAPIState(noFlags)
	cfgEndpointOpened := make(chan struct{})

	s.PatchValue(juju.ProviderConnectDelay, 0*time.Second)
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		if info.Addrs[0] == "0.1.2.3" {
			infoEndpointOpened <- struct{}{}
			<-infoEndpointOpened
			return infoOpenedState, nil
		}
		cfgEndpointOpened <- struct{}{}
		<-cfgEndpointOpened
		return cfgOpenedState, nil
	}

	stateClosed := make(chan api.Connection)
	infoOpenedState.close = func(st api.Connection) error {
		stateClosed <- st
		return nil
	}
	cfgOpenedState.close = infoOpenedState.close

	done := make(chan struct{})
	go func() {
		st, err := juju.NewAPIFromStore(coretesting.SampleEnvName, store, apiOpen)
		c.Check(err, jc.ErrorIsNil)
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
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	coretesting.MakeSampleJujuHome(c)
	store := configstore.NewMem()
	s.bootstrapEnv(c, coretesting.SampleEnvName, store)
	setEndpointAddressAndHostname(c, store, coretesting.SampleEnvName, "0.1.2.3", "infoapi.invalid")

	s.PatchValue(juju.ProviderConnectDelay, 0*time.Second)
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
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
	c.Assert(err, jc.ErrorIsNil)
	return store
}

func (s *NewAPIClientSuite) TestWithBootstrapConfigAndNoEnvironmentsFile(c *gc.C) {
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	coretesting.MakeSampleJujuHome(c)
	store := configstore.NewMem()
	s.bootstrapEnv(c, coretesting.SampleEnvName, store)
	info, err := store.ReadInfo(coretesting.SampleEnvName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.BootstrapConfig(), gc.NotNil)
	c.Assert(info.APIEndpoint().Addresses, gc.HasLen, 0)

	err = os.Remove(osenv.JujuHomePath("environments.yaml"))
	c.Assert(err, jc.ErrorIsNil)

	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return mockedAPIState(noFlags), nil
	}
	st, err := juju.NewAPIFromStore(coretesting.SampleEnvName, store, apiOpen)
	c.Check(err, jc.ErrorIsNil)
	st.Close()
}

func (s *NewAPIClientSuite) TestWithBootstrapConfigTakesPrecedence(c *gc.C) {
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	// We want to make sure that the code is using the bootstrap
	// config rather than information from environments.yaml,
	// even when there is an entry in environments.yaml
	// We can do that by changing the info bootstrap config
	// so it has a different environment name.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)

	store := configstore.NewMem()
	s.bootstrapEnv(c, coretesting.SampleEnvName, store)
	info, err := store.ReadInfo(coretesting.SampleEnvName)
	c.Assert(err, jc.ErrorIsNil)

	envName2 := coretesting.SampleCertName + "-2"
	info2 := store.CreateInfo(envName2)
	info2.SetBootstrapConfig(info.BootstrapConfig())
	err = info2.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Now we have info for envName2 which will actually
	// cause a connection to the originally bootstrapped
	// state.
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return mockedAPIState(noFlags), nil
	}
	st, err := juju.NewAPIFromStore(envName2, store, apiOpen)
	c.Check(err, jc.ErrorIsNil)
	st.Close()

	// Sanity check that connecting to the envName2
	// but with no info fails.
	// Currently this panics with an "environment not prepared" error.
	// Disable for now until an upcoming branch fixes it.
	//	err = info2.Destroy()
	//	c.Assert(err, jc.ErrorIsNil)
	//	st, err = juju.NewAPIFromStore(envName2, store)
	//	if err == nil {
	//		st.Close()
	//	}
	//	c.Assert(err, gc.ErrorMatches, "fooobie")
}

func assertEnvironmentName(c *gc.C, client *api.Client, expectName string) {
	envInfo, err := client.EnvironmentInfo()
	c.Assert(err, jc.ErrorIsNil)
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
	newInfo := store.CreateInfo(envName)
	newInfo.SetAPICredentials(info.creds)
	newInfo.SetAPIEndpoint(info.endpoint)
	newInfo.SetBootstrapConfig(info.bootstrapConfig)
	err := newInfo.Write()
	if err != nil {
		panic(err)
	}
	return store
}

type storageWithWriteNotify struct {
	written bool
	store   configstore.Storage
}

func (*storageWithWriteNotify) CreateInfo(envName string) configstore.EnvironInfo {
	panic("CreateInfo not implemented")
}

func (*storageWithWriteNotify) List() ([]string, error) {
	panic("List not implemented")
}

func (*storageWithWriteNotify) ListSystems() ([]string, error) {
	panic("ListSystems not implemented")
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

type CacheAPIEndpointsSuite struct {
	jujutesting.JujuConnSuite

	hostPorts   [][]network.HostPort
	envTag      names.EnvironTag
	apiHostPort network.HostPort
	store       configstore.Storage

	resolveSeq      int
	resolveNumCalls int
	numResolved     int
	gocheckC        *gc.C
}

var _ = gc.Suite(&CacheAPIEndpointsSuite{})

func (s *CacheAPIEndpointsSuite) SetUpTest(c *gc.C) {
	s.PatchValue(juju.ResolveOrDropHostnames, s.mockResolveOrDropHostnames)

	s.hostPorts = [][]network.HostPort{
		network.NewHostPorts(1234,
			"1.0.0.1",
			"192.0.0.1",
			"127.0.0.1",
			"ipv4+6.example.com",
			"localhost",
			"169.254.1.1",
			"ipv4.example.com",
			"invalid host",
			"ipv6+6.example.com",
			"ipv4+4.example.com",
			"::1",
			"fe80::1",
			"ipv6.example.com",
			"fc00::111",
			"2001:db8::1",
		),
		network.NewHostPorts(1235,
			"1.0.0.2",
			"2001:db8::2",
			"::1",
			"127.0.0.1",
			"ipv6+4.example.com",
			"localhost",
		),
	}
	s.gocheckC = c
	s.resolveSeq = 1
	s.resolveNumCalls = 0
	s.numResolved = 0
	s.envTag = names.NewEnvironTag(fakeUUID)
	s.store = configstore.NewMem()

	s.JujuConnSuite.SetUpTest(c)

	apiHostPort, err := network.ParseHostPorts(s.APIState.Addr())
	c.Assert(err, jc.ErrorIsNil)
	s.apiHostPort = apiHostPort[0]
}

func (s *CacheAPIEndpointsSuite) TestPrepareEndpointsForCachingPreferIPv6True(c *gc.C) {
	info := s.store.CreateInfo("env-name1")
	s.PatchValue(juju.MaybePreferIPv6, func(_ configstore.EnvironInfo) bool {
		return true
	})
	// First test cacheChangedAPIInfo behaves as expected.
	err := juju.CacheChangedAPIInfo(info, s.hostPorts, s.apiHostPort, s.envTag.Id(), "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpointsPreferIPv6True(c, info)

	// Now test cacheAPIInfo behaves the same way.
	s.resolveSeq = 1
	s.resolveNumCalls = 0
	s.numResolved = 0
	info = s.store.CreateInfo("env-name2")
	mockAPIInfo := s.APIInfo(c)
	mockAPIInfo.EnvironTag = s.envTag
	hps := network.CollapseHostPorts(s.hostPorts)
	mockAPIInfo.Addrs = network.HostPortsToStrings(hps)
	err = juju.CacheAPIInfo(s.APIState, info, mockAPIInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpointsPreferIPv6True(c, info)
}

func (s *CacheAPIEndpointsSuite) TestPrepareEndpointsForCachingPreferIPv6False(c *gc.C) {
	info := s.store.CreateInfo("env-name1")
	s.PatchValue(juju.MaybePreferIPv6, func(_ configstore.EnvironInfo) bool {
		return false
	})
	// First test cacheChangedAPIInfo behaves as expected.
	err := juju.CacheChangedAPIInfo(info, s.hostPorts, s.apiHostPort, s.envTag.Id(), "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpointsPreferIPv6False(c, info)

	// Now test cacheAPIInfo behaves the same way.
	s.resolveSeq = 1
	s.resolveNumCalls = 0
	s.numResolved = 0
	info = s.store.CreateInfo("env-name2")
	mockAPIInfo := s.APIInfo(c)
	mockAPIInfo.EnvironTag = s.envTag
	hps := network.CollapseHostPorts(s.hostPorts)
	mockAPIInfo.Addrs = network.HostPortsToStrings(hps)
	err = juju.CacheAPIInfo(s.APIState, info, mockAPIInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpointsPreferIPv6False(c, info)
}

func (s *CacheAPIEndpointsSuite) TestResolveSkippedWhenHostnamesUnchanged(c *gc.C) {
	// Test that if new endpoints hostnames are the same as the
	// cached, no DNS resolution happens (i.e. we don't resolve on
	// every connection, but as needed).
	info := s.store.CreateInfo("env-name")
	hps := network.NewHostPorts(1234,
		"8.8.8.8",
		"example.com",
		"10.0.0.1",
	)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Hostnames: network.HostPortsToStrings(hps),
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		info, [][]network.HostPort{hps}, network.HostPort{},
	)
	c.Assert(addrs, gc.IsNil)
	c.Assert(hosts, gc.IsNil)
	c.Assert(changed, jc.IsFalse)
	c.Assert(s.resolveNumCalls, gc.Equals, 0)
	c.Assert(
		c.GetTestLog(),
		jc.Contains,
		"DEBUG juju.api API hostnames unchanged - not resolving",
	)
}

func (s *CacheAPIEndpointsSuite) TestResolveCalledWithChangedHostnames(c *gc.C) {
	// Test that if new endpoints hostnames are different than the
	// cached hostnames DNS resolution happens and we compare resolved
	// addresses.
	info := s.store.CreateInfo("env-name")
	// Because Hostnames are sorted before caching, reordering them
	// will simulate they have changed.
	unsortedHPs := network.NewHostPorts(1234,
		"ipv4.example.com",
		"8.8.8.8",
		"ipv6.example.com",
		"10.0.0.1",
	)
	strUnsorted := network.HostPortsToStrings(unsortedHPs)
	sortedHPs := network.NewHostPorts(1234,
		"8.8.8.8",
		"ipv4.example.com",
		"ipv6.example.com",
		"10.0.0.1",
	)
	strSorted := network.HostPortsToStrings(sortedHPs)
	resolvedHPs := network.NewHostPorts(1234,
		"0.1.2.1", // from ipv4.example.com
		"8.8.8.8",
		"10.0.0.1",
		"fc00::2", // from ipv6.example.com
	)
	strResolved := network.HostPortsToStrings(resolvedHPs)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Hostnames: strUnsorted,
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		info, [][]network.HostPort{unsortedHPs}, network.HostPort{},
	)
	c.Assert(addrs, jc.DeepEquals, strResolved)
	c.Assert(hosts, jc.DeepEquals, strSorted)
	c.Assert(changed, jc.IsTrue)
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 2)
	expectLog := fmt.Sprintf("DEBUG juju.api API hostnames changed from %v to %v - resolving hostnames", unsortedHPs, sortedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
	expectLog = fmt.Sprintf("INFO juju.api new API addresses to cache %v", resolvedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
}

func (s *CacheAPIEndpointsSuite) TestAfterResolvingUnchangedAddressesNotCached(c *gc.C) {
	// Test that if new endpoints hostnames are different than the
	// cached hostnames, but after resolving the addresses match the
	// cached addresses, the cache is not changed.
	info := s.store.CreateInfo("env-name")
	// Because Hostnames are sorted before caching, reordering them
	// will simulate they have changed.
	unsortedHPs := network.NewHostPorts(1234,
		"ipv4.example.com",
		"8.8.8.8",
		"ipv6.example.com",
		"10.0.0.1",
	)
	strUnsorted := network.HostPortsToStrings(unsortedHPs)
	sortedHPs := network.NewHostPorts(1234,
		"8.8.8.8",
		"ipv4.example.com",
		"ipv6.example.com",
		"10.0.0.1",
	)
	resolvedHPs := network.NewHostPorts(1234,
		"0.1.2.1", // from ipv4.example.com
		"8.8.8.8",
		"10.0.0.1",
		"fc00::2", // from ipv6.example.com
	)
	strResolved := network.HostPortsToStrings(resolvedHPs)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Hostnames: strUnsorted,
		Addresses: strResolved,
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		info, [][]network.HostPort{unsortedHPs}, network.HostPort{},
	)
	c.Assert(addrs, gc.IsNil)
	c.Assert(hosts, gc.IsNil)
	c.Assert(changed, jc.IsFalse)
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 2)
	expectLog := fmt.Sprintf("DEBUG juju.api API hostnames changed from %v to %v - resolving hostnames", unsortedHPs, sortedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
	expectLog = "DEBUG juju.api API addresses unchanged"
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
}

func (s *CacheAPIEndpointsSuite) TestResolveCalledWithInitialEndpoints(c *gc.C) {
	// Test that if no hostnames exist cached we call resolve (i.e.
	// simulate the behavior right after bootstrap)
	info := s.store.CreateInfo("env-name")
	// Because Hostnames are sorted before caching, reordering them
	// will simulate they have changed.
	unsortedHPs := network.NewHostPorts(1234,
		"ipv4.example.com",
		"8.8.8.8",
		"ipv6.example.com",
		"10.0.0.1",
	)
	sortedHPs := network.NewHostPorts(1234,
		"8.8.8.8",
		"ipv4.example.com",
		"ipv6.example.com",
		"10.0.0.1",
	)
	strSorted := network.HostPortsToStrings(sortedHPs)
	resolvedHPs := network.NewHostPorts(1234,
		"0.1.2.1", // from ipv4.example.com
		"8.8.8.8",
		"10.0.0.1",
		"fc00::2", // from ipv6.example.com
	)
	strResolved := network.HostPortsToStrings(resolvedHPs)
	info.SetAPIEndpoint(configstore.APIEndpoint{})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		info, [][]network.HostPort{unsortedHPs}, network.HostPort{},
	)
	c.Assert(addrs, jc.DeepEquals, strResolved)
	c.Assert(hosts, jc.DeepEquals, strSorted)
	c.Assert(changed, jc.IsTrue)
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 2)
	expectLog := fmt.Sprintf("DEBUG juju.api API hostnames %v - resolving hostnames", sortedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
	expectLog = fmt.Sprintf("INFO juju.api new API addresses to cache %v", resolvedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
}

func (s *CacheAPIEndpointsSuite) assertEndpointsPreferIPv6False(c *gc.C, info configstore.EnvironInfo) {
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 10)
	endpoint := info.APIEndpoint()
	// Check Addresses after resolving.
	c.Check(endpoint.Addresses, jc.DeepEquals, []string{
		s.apiHostPort.NetAddr(), // Last endpoint successfully connected to is always on top.
		"0.1.2.1:1234",          // From ipv4+4.example.com
		"0.1.2.2:1234",          // From ipv4+4.example.com
		"0.1.2.3:1234",          // From ipv4+6.example.com
		"0.1.2.5:1234",          // From ipv4.example.com
		"0.1.2.6:1234",          // From ipv6+4.example.com
		"1.0.0.1:1234",
		"1.0.0.2:1235",
		"192.0.0.1:1234",
		"[2001:db8::1]:1234",
		"[2001:db8::2]:1235",
		"localhost:1234",  // Left intact on purpose.
		"localhost:1235",  // Left intact on purpose.
		"[fc00::10]:1234", // From ipv6.example.com
		"[fc00::111]:1234",
		"[fc00::3]:1234", // From ipv4+6.example.com
		"[fc00::6]:1234", // From ipv6+4.example.com
		"[fc00::8]:1234", // From ipv6+6.example.com
		"[fc00::9]:1234", // From ipv6+6.example.com
	})
	// Check Hostnames before resolving
	c.Check(endpoint.Hostnames, jc.DeepEquals, []string{
		s.apiHostPort.NetAddr(), // Last endpoint successfully connected to is always on top.
		"1.0.0.1:1234",
		"1.0.0.2:1235",
		"192.0.0.1:1234",
		"[2001:db8::1]:1234",
		"[2001:db8::2]:1235",
		"invalid host:1234",
		"ipv4+4.example.com:1234",
		"ipv4+6.example.com:1234",
		"ipv4.example.com:1234",
		"ipv6+4.example.com:1235",
		"ipv6+6.example.com:1234",
		"ipv6.example.com:1234",
		"localhost:1234",
		"localhost:1235",
		"[fc00::111]:1234",
	})
}

func (s *CacheAPIEndpointsSuite) assertEndpointsPreferIPv6True(c *gc.C, info configstore.EnvironInfo) {
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 10)
	endpoint := info.APIEndpoint()
	// Check Addresses after resolving.
	c.Check(endpoint.Addresses, jc.DeepEquals, []string{
		s.apiHostPort.NetAddr(), // Last endpoint successfully connected to is always on top.
		"[2001:db8::1]:1234",
		"[2001:db8::2]:1235",
		"0.1.2.1:1234", // From ipv4+4.example.com
		"0.1.2.2:1234", // From ipv4+4.example.com
		"0.1.2.3:1234", // From ipv4+6.example.com
		"0.1.2.5:1234", // From ipv4.example.com
		"0.1.2.6:1234", // From ipv6+4.example.com
		"1.0.0.1:1234",
		"1.0.0.2:1235",
		"192.0.0.1:1234",
		"localhost:1234",  // Left intact on purpose.
		"localhost:1235",  // Left intact on purpose.
		"[fc00::10]:1234", // From ipv6.example.com
		"[fc00::111]:1234",
		"[fc00::3]:1234", // From ipv4+6.example.com
		"[fc00::6]:1234", // From ipv6+4.example.com
		"[fc00::8]:1234", // From ipv6+6.example.com
		"[fc00::9]:1234", // From ipv6+6.example.com
	})
	// Check Hostnames before resolving
	c.Check(endpoint.Hostnames, jc.DeepEquals, []string{
		s.apiHostPort.NetAddr(), // Last endpoint successfully connected to is always on top.
		"[2001:db8::1]:1234",
		"[2001:db8::2]:1235",
		"1.0.0.1:1234",
		"1.0.0.2:1235",
		"192.0.0.1:1234",
		"invalid host:1234",
		"ipv4+4.example.com:1234",
		"ipv4+6.example.com:1234",
		"ipv4.example.com:1234",
		"ipv6+4.example.com:1235",
		"ipv6+6.example.com:1234",
		"ipv6.example.com:1234",
		"localhost:1234",
		"localhost:1235",
		"[fc00::111]:1234",
	})
}

func (s *CacheAPIEndpointsSuite) nextHostPorts(host string, types ...network.AddressType) []network.HostPort {
	result := make([]network.HostPort, len(types))
	num4, num6 := 0, 0
	for i, tp := range types {
		addr := ""
		switch tp {
		case network.IPv4Address:
			addr = fmt.Sprintf("0.1.2.%d", s.resolveSeq+num4)
			num4++
		case network.IPv6Address:
			addr = fmt.Sprintf("fc00::%d", s.resolveSeq+num6)
			num6++
		}
		result[i] = network.NewHostPorts(1234, addr)[0]
	}
	s.resolveSeq += num4 + num6
	s.gocheckC.Logf("resolving %q as %v", host, result)
	return result
}

func (s *CacheAPIEndpointsSuite) mockResolveOrDropHostnames(hps []network.HostPort) []network.HostPort {
	s.resolveNumCalls++
	var result []network.HostPort
	for _, hp := range hps {
		if hp.Value == "invalid host" || hp.Scope == network.ScopeLinkLocal {
			// Simulate we dropped this.
			continue
		} else if hp.Value == "localhost" || hp.Type != network.HostName {
			// Leave localhost and IPs alone.
			result = append(result, hp)
			continue
		}
		var types []network.AddressType
		switch strings.TrimSuffix(hp.Value, ".example.com") {
		case "ipv4":
			// Simulate it resolves to an IPv4 address.
			types = append(types, network.IPv4Address)
		case "ipv6":
			// Simulate it resolves to an IPv6 address.
			types = append(types, network.IPv6Address)
		case "ipv4+6":
			// Simulate it resolves to both IPv4 and IPv6 addresses.
			types = append(types, network.IPv4Address, network.IPv6Address)
		case "ipv6+6":
			// Simulate it resolves to two IPv6 addresses.
			types = append(types, network.IPv6Address, network.IPv6Address)
		case "ipv4+4":
			// Simulate it resolves to two IPv4 addresses.
			types = append(types, network.IPv4Address, network.IPv4Address)
		case "ipv6+4":
			// Simulate it resolves to both IPv4 and IPv6 addresses.
			types = append(types, network.IPv6Address, network.IPv4Address)
		}
		result = append(result, s.nextHostPorts(hp.Value, types...)...)
		s.numResolved += len(types)
	}
	return result
}

var fakeUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

var dummyStoreInfo = &environInfo{
	creds: configstore.APICredentials{
		User:     "foo",
		Password: "foopass",
	},
	endpoint: configstore.APIEndpoint{
		Addresses:   []string{"foo.invalid"},
		CACert:      "certificated",
		EnvironUUID: fakeUUID,
	},
}

type EnvironInfoTest struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&EnvironInfoTest{})

func (*EnvironInfoTest) TestNullInfo(c *gc.C) {
	c.Assert(juju.EnvironInfoUserTag(nil), gc.Equals, names.NewUserTag(configstore.DefaultAdminUsername))
}

type fakeEnvironInfo struct {
	configstore.EnvironInfo
	user string
}

func (fake *fakeEnvironInfo) APICredentials() configstore.APICredentials {
	return configstore.APICredentials{User: fake.user}
}

func (*EnvironInfoTest) TestEmptyUser(c *gc.C) {
	info := &fakeEnvironInfo{}
	c.Assert(juju.EnvironInfoUserTag(info), gc.Equals, names.NewUserTag(configstore.DefaultAdminUsername))
}

func (*EnvironInfoTest) TestRealUser(c *gc.C) {
	info := &fakeEnvironInfo{user: "eric"}
	c.Assert(juju.EnvironInfoUserTag(info), gc.Equals, names.NewUserTag("eric"))
}
