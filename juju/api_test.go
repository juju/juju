// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"
	"net"
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
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type NewAPIClientSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testing.MgoSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&NewAPIClientSuite{})

func (cs *NewAPIClientSuite) SetUpSuite(c *gc.C) {
	cs.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	cs.MgoSuite.SetUpSuite(c)
	cs.PatchValue(&simplestreams.SimplestreamsJujuPublicKey, sstesting.SignedMetadataPublicKey)
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
	cs.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (cs *NewAPIClientSuite) SetUpTest(c *gc.C) {
	cs.ToolsFixture.SetUpTest(c)
	cs.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
	cs.PatchValue(&dummy.LogDir, c.MkDir())
}

func (cs *NewAPIClientSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.ToolsFixture.TearDownTest(c)
	cs.MgoSuite.TearDownTest(c)
	cs.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *NewAPIClientSuite) bootstrapEnv(c *gc.C) (configstore.Storage, jujuclient.ClientStore) {
	const controllerName = "local.my-controller"

	legacyStore := configstore.NewMem()
	store := jujuclienttesting.NewMemStore()

	ctx := envtesting.BootstrapContext(c)
	cfg, err := config.New(config.UseDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(ctx, legacyStore, store, controllerName, environs.PrepareForBootstrapParams{Config: cfg})
	c.Assert(err, jc.ErrorIsNil)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	return legacyStore, store
}

func (s *NewAPIClientSuite) TestWithInfoOnly(c *gc.C) {
	legacyStore := newConfigStore("noconfig", dummyStoreInfo)
	store := newClientStore(c, "noconfig", dummyStoreInfo)

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.ModelTag, gc.Equals, names.NewModelTag(fakeUUID))
		called++
		return expectState, nil
	}

	// Give NewAPIFromStore a store interface that can report when the
	// config was written to, to check if the cache is updated.
	mockStore := &storageWithWriteNotify{store: legacyStore}
	st, err := juju.NewAPIFromStore("noconfig", "admin@local", "noconfig", mockStore, store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	c.Assert(mockStore.written, jc.IsTrue)
	info, err := legacyStore.ReadInfo("noconfig:noconfig")
	c.Assert(err, jc.ErrorIsNil)
	ep := info.APIEndpoint()
	c.Check(ep.Addresses, jc.DeepEquals, []string{
		"0.1.2.3:1234", "[2001:db8::1]:1234",
	})
	c.Check(ep.ModelUUID, gc.Equals, fakeUUID)
	mockStore.written = false

	controllerBefore, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)

	// If APIHostPorts haven't changed, then the store won't be updated.
	st, err = juju.NewAPIFromStore("noconfig", "admin@local", "noconfig", mockStore, store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 2)
	c.Assert(mockStore.written, jc.IsFalse)

	controllerAfter, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerBefore, gc.DeepEquals, controllerAfter)
}

func (s *NewAPIClientSuite) TestWithInfoError(c *gc.C) {
	expectErr := fmt.Errorf("an error")
	legacyStore := newConfigStoreWithError(expectErr)
	store := newClientStore(c, "noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			ServerUUID: "valid.uuid",
			CACert:     "certificated",
		},
	})
	client, err := juju.NewAPIFromStore("noconfig", "", "", legacyStore, store, panicAPIOpen)
	c.Assert(errors.Cause(err), gc.Equals, expectErr)
	c.Assert(client, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithInfoNoAddresses(c *gc.C) {
	store := newConfigStore("noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			Addresses: []string{},
			CACert:    "certificated",
		},
	})
	cache := newClientStore(c, "noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			Addresses:  []string{},
			ServerUUID: "valid.uuid",
			ModelUUID:  "valid.uuid",
			CACert:     "certificated",
		},
	})
	st, err := juju.NewAPIFromStore("noconfig", "admin@local", "noconfig", store, cache, panicAPIOpen)
	c.Assert(err, gc.ErrorMatches, "bootstrap config not found")
	c.Assert(st, gc.IsNil)
}

var noTagStoreInfo = &environInfo{
	creds: configstore.APICredentials{
		User:     "admin@local",
		Password: "hunter2",
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
	mockedModelTag   mockedStateFlags = 0x0002
	mockedPreferIPv6 mockedStateFlags = 0x0004
)

func mockedAPIState(flags mockedStateFlags) *mockAPIState {
	hasHostPort := flags&mockedHostPort == mockedHostPort
	hasModelTag := flags&mockedModelTag == mockedModelTag
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
	modelTag := ""
	if hasModelTag {
		modelTag = "model-df136476-12e9-11e4-8a70-b2227cce2b54"
	}
	return &mockAPIState{
		apiHostPorts:  apiHostPorts,
		modelTag:      modelTag,
		controllerTag: modelTag,
		addr:          addr,
	}
}

func checkCommonAPIInfoAttrs(c *gc.C, apiInfo *api.Info, opts api.DialOpts) {
	c.Check(apiInfo.Tag, gc.Equals, names.NewUserTag("admin@local"))
	c.Check(string(apiInfo.CACert), gc.Equals, "certificated")
	c.Check(apiInfo.Password, gc.Equals, "hunter2")
	c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
}

func (s *NewAPIClientSuite) TestWithInfoNoAPIHostports(c *gc.C) {
	// The API doesn't have apiHostPorts, we don't want to
	// override the local cache with bad endpoints.
	legacyStore := newConfigStore("noconfig", noTagStoreInfo)
	store := newClientStore(c, "noconfig", dummyStoreInfo)

	called := 0
	expectState := mockedAPIState(mockedModelTag | mockedPreferIPv6)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.ModelTag.Id(), gc.Equals, "")
		called++
		return expectState, nil
	}

	mockStore := &storageWithWriteNotify{store: legacyStore}
	st, err := juju.NewAPIFromStore("noconfig", "admin@local", "", mockStore, store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)

	// We should not have disturbed the Addresses

	details, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(details.APIEndpoints, gc.HasLen, 1)
	c.Check(details.APIEndpoints[0], gc.Matches, `foo\.invalid`)

	info, err := legacyStore.ReadInfo("noconfig:noconfig")
	c.Assert(err, jc.ErrorIsNil)
	ep := info.APIEndpoint()
	c.Check(ep.Addresses, gc.HasLen, 1)
	c.Check(ep.Addresses[0], gc.Matches, `foo\.invalid`)
}

func (s *NewAPIClientSuite) TestWithInfoAPIOpenError(c *gc.C) {
	store := newConfigStore("noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			Addresses: []string{"foo.invalid"},
		},
	})
	jujuClient := newClientStore(c, "noconfig", &environInfo{
		endpoint: configstore.APIEndpoint{
			Addresses:  []string{"foo.invalid"},
			ServerUUID: "some.uuid",
			CACert:     "some.cert",
		},
	})

	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		return nil, errors.Errorf("an error")
	}
	st, err := juju.NewAPIFromStore("noconfig", "", "", store, jujuClient, apiOpen)
	// We expect to  get the isNotFound error as it is more important than the
	// infoConnectError "an error"
	c.Assert(err, gc.ErrorMatches, "bootstrap config not found")
	c.Assert(st, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithSlowInfoConnect(c *gc.C) {
	c.Skip("wallyworld - this is a dumb test relying on an arbitary 50ms delay to pass")
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	legacyStore, store := s.bootstrapEnv(c)
	setEndpointAddressAndHostname(c, store, "0.1.2.3", "infoapi.invalid")

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
	st, err := juju.NewAPIFromStore("local.my-controller", "admin@local", "only", legacyStore, store, apiOpen)
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

func (s *NewAPIClientSuite) TestGetBootstrapConfigNoLegacyInfo(c *gc.C) {
	store := configstore.NewMem()
	cfg, err := juju.GetBootstrapConfig(store, "whatever", "")
	c.Assert(err, gc.ErrorMatches, `getting controller info: model "whatever:whatever" not found`)
	c.Assert(cfg, gc.IsNil)
}

func (s *NewAPIClientSuite) TestGetBootstrapConfigNoBootstrapConfig(c *gc.C) {
	store := configstore.NewMem()
	info := store.CreateInfo("whatever:whatever")
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := juju.GetBootstrapConfig(store, "whatever", "")
	c.Assert(err, gc.ErrorMatches, "bootstrap config not found")
	c.Assert(cfg, gc.IsNil)
}

func (s *NewAPIClientSuite) TestGetBootstrapConfigBadConfigDoesntPanic(c *gc.C) {
	store := configstore.NewMem()
	info := store.CreateInfo("whatever:whatever")
	info.SetBootstrapConfig(map[string]interface{}{"something": "else"})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := juju.GetBootstrapConfig(store, "whatever", "")
	// The specific error we get depends on what key is invalid, which is a
	// bit spurious, but what we care about is that we didn't get a panic,
	// but instead got an error
	c.Assert(err, gc.ErrorMatches, ".*expected.*got nothing")
	c.Assert(cfg, gc.IsNil)
}

func setEndpointAddressAndHostname(c *gc.C, store jujuclient.ControllerStore, addr, host string) {
	// Populate the controller details with known address and hostname.
	details, err := store.ControllerByName("local.my-controller")
	c.Assert(err, jc.ErrorIsNil)
	details.APIEndpoints = []string{addr}
	details.Servers = []string{host}
	err = store.UpdateController("local.my-controller", *details)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NewAPIClientSuite) TestWithSlowConfigConnect(c *gc.C) {
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)

	legacyStore, store := s.bootstrapEnv(c)
	setEndpointAddressAndHostname(c, store, "0.1.2.3", "infoapi.invalid")

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
		st, err := juju.NewAPIFromStore("local.my-controller", "admin@local", "only", legacyStore, store, apiOpen)
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
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	legacyStore, store := s.bootstrapEnv(c)
	setEndpointAddressAndHostname(c, store, "0.1.2.3", "infoapi.invalid")

	s.PatchValue(juju.ProviderConnectDelay, 0*time.Second)
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		if info.Addrs[0] == "infoapi.invalid" {
			return nil, fmt.Errorf("info connect failed")
		}
		return nil, fmt.Errorf("config connect failed")
	}
	st, err := juju.NewAPIFromStore("local.my-controller", "admin@local", "only", legacyStore, store, apiOpen)
	c.Check(err, gc.ErrorMatches, "config connect failed")
	c.Check(st, gc.IsNil)
}

func defaultConfigStore(c *gc.C) configstore.Storage {
	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	return store
}

func (s *NewAPIClientSuite) TestWithBootstrapConfigAndNoEnvironmentsFile(c *gc.C) {
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	legacyStore, store := s.bootstrapEnv(c)

	details, err := store.ControllerByName("local.my-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.APIEndpoints, gc.HasLen, 0)

	info, err := legacyStore.ReadInfo("local.my-controller:only")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.BootstrapConfig(), gc.NotNil)
	c.Assert(info.APIEndpoint().Addresses, gc.HasLen, 0)

	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return mockedAPIState(noFlags), nil
	}
	st, err := juju.NewAPIFromStore("local.my-controller", "admin@local", "only", legacyStore, store, apiOpen)
	c.Check(err, jc.ErrorIsNil)
	st.Close()
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
	newInfo := store.CreateInfo(envName + ":" + envName)
	newInfo.SetAPICredentials(info.creds)
	newInfo.SetAPIEndpoint(info.endpoint)
	newInfo.SetBootstrapConfig(info.bootstrapConfig)
	err := newInfo.Write()
	if err != nil {
		panic(err)
	}
	return store
}

// newClientStore returns a client store that contains information
// based on the given controller namd and info.
func newClientStore(c *gc.C, controllerName string, info *environInfo) jujuclient.ClientStore {
	store := jujuclienttesting.NewMemStore()
	err := store.UpdateController(controllerName, jujuclient.ControllerDetails{
		info.endpoint.Hostnames,
		info.endpoint.ServerUUID,
		info.endpoint.Addresses,
		info.endpoint.CACert,
	})
	c.Assert(err, jc.ErrorIsNil)

	if info.endpoint.ModelUUID != "" {
		err = store.UpdateModel(controllerName, "admin@local", controllerName, jujuclient.ModelDetails{
			info.endpoint.ModelUUID,
		})
		c.Assert(err, jc.ErrorIsNil)

		// Models belong to accounts, so we must have an account even
		// if "creds" is not initialised. If it is, it may overwrite
		// this one.
		err = store.UpdateAccount(controllerName, "admin@local", jujuclient.AccountDetails{
			User: "admin@local",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = store.SetCurrentAccount(controllerName, "admin@local")
		c.Assert(err, jc.ErrorIsNil)
	}

	if info.creds.User != "" {
		user := names.NewUserTag(info.creds.User).Canonical()
		err = store.UpdateAccount(controllerName, user, jujuclient.AccountDetails{
			User:     user,
			Password: info.creds.Password,
		})
		c.Assert(err, jc.ErrorIsNil)
		err = store.SetCurrentAccount(controllerName, user)
		c.Assert(err, jc.ErrorIsNil)
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
	return nil, nil
}

func (*storageWithWriteNotify) ListSystems() ([]string, error) {
	return []string{"noconfig:noconfig"}, nil
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
	modelTag    names.ModelTag
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
	s.modelTag = names.NewModelTag(fakeUUID)
	s.store = configstore.NewMem()

	s.JujuConnSuite.SetUpTest(c)

	apiHostPort, err := network.ParseHostPorts(s.APIState.Addr())
	c.Assert(err, jc.ErrorIsNil)
	s.apiHostPort = apiHostPort[0]
}

func (s *CacheAPIEndpointsSuite) assertCreateInfo(c *gc.C, name string) configstore.EnvironInfo {
	info := s.store.CreateInfo(name)

	// info should have server uuid.
	updateEndpoint := info.APIEndpoint()
	updateEndpoint.ServerUUID = fakeUUID
	info.SetAPIEndpoint(updateEndpoint)

	// write controller
	c.Assert(updateEndpoint.Hostnames, gc.HasLen, 0)
	c.Assert(updateEndpoint.Addresses, gc.HasLen, 0)
	controllerDetails := jujuclient.ControllerDetails{
		updateEndpoint.Hostnames,
		fakeUUID,
		updateEndpoint.Addresses,
		"this.is.ca.cert.but.not.relevant.slash.used.in.this.test",
	}
	err := s.ControllerStore.UpdateController(name, controllerDetails)
	c.Assert(err, jc.ErrorIsNil)
	return info
}

func (s *CacheAPIEndpointsSuite) assertControllerDetailsUpdated(c *gc.C, name string, check gc.Checker) {
	found, err := s.ControllerStore.ControllerByName(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Servers, check, 0)
	c.Assert(found.APIEndpoints, check, 0)
}

func (s *CacheAPIEndpointsSuite) assertControllerUpdated(c *gc.C, name string) {
	s.assertControllerDetailsUpdated(c, name, gc.Not(gc.HasLen))
}

func (s *CacheAPIEndpointsSuite) assertControllerNotUpdated(c *gc.C, name string) {
	s.assertControllerDetailsUpdated(c, name, gc.HasLen)
}

func (s *CacheAPIEndpointsSuite) TestPrepareEndpointsForCachingPreferIPv6True(c *gc.C) {
	s.PatchValue(juju.MaybePreferIPv6, func(_ configstore.EnvironInfo) bool {
		return true
	})

	info := s.assertCreateInfo(c, "controller-name1")
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	err = juju.UpdateControllerAddresses(s.ControllerStore, s.store, "controller-name1", s.hostPorts, s.apiHostPort)
	c.Assert(err, jc.ErrorIsNil)
	info, err = s.store.ReadInfo("controller-name1")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpointsPreferIPv6True(c, info)
	s.assertControllerUpdated(c, "controller-name1")
}

func (s *CacheAPIEndpointsSuite) TestPrepareEndpointsForCachingPreferIPv6False(c *gc.C) {
	s.PatchValue(juju.MaybePreferIPv6, func(_ configstore.EnvironInfo) bool {
		return false
	})
	info := s.assertCreateInfo(c, "controller-name1")
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	err = juju.UpdateControllerAddresses(s.ControllerStore, s.store, "controller-name1", s.hostPorts, s.apiHostPort)
	c.Assert(err, jc.ErrorIsNil)
	info, err = s.store.ReadInfo("controller-name1")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpointsPreferIPv6False(c, info)
	s.assertControllerUpdated(c, "controller-name1")
}

func (s *CacheAPIEndpointsSuite) TestResolveSkippedWhenHostnamesUnchanged(c *gc.C) {
	// Test that if new endpoints hostnames are the same as the
	// cached, no DNS resolution happens (i.e. we don't resolve on
	// every connection, but as needed).
	info := s.store.CreateInfo("controller-name")
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
		info, [][]network.HostPort{hps},
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
	info := s.store.CreateInfo("controller-name")
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
		info, [][]network.HostPort{unsortedHPs},
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
	info := s.store.CreateInfo("controller-name")
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
		info, [][]network.HostPort{unsortedHPs},
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
	info := s.store.CreateInfo("controller-name")
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
		info, [][]network.HostPort{unsortedHPs},
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
		User:     "admin@local",
		Password: "hunter2",
	},
	endpoint: configstore.APIEndpoint{
		Addresses:  []string{"foo.invalid"},
		CACert:     "certificated",
		ModelUUID:  fakeUUID,
		ServerUUID: fakeUUID,
	},
}
