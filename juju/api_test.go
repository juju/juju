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
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
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
	jujuversion "github.com/juju/juju/version"
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
	cs.PatchValue(&juju.JujuPublicKey, sstesting.SignedMetadataPublicKey)
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

func (s *NewAPIClientSuite) bootstrapModel(c *gc.C) (environs.Environ, jujuclient.ClientStore) {
	const controllerName = "local.my-controller"

	store := jujuclienttesting.NewMemStore()

	ctx := envtesting.BootstrapContext(c)

	env, err := environs.Prepare(ctx, store, environs.PrepareParams{
		ControllerName: controllerName,
		BaseConfig:     dummy.SampleConfig(),
		CloudName:      "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	return env, store
}

func (s *NewAPIClientSuite) TestWithBootstrapConfig(c *gc.C) {
	store := newClientStore(c, "noconfig")

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.ModelTag, gc.Equals, names.NewModelTag(fakeUUID))
		called++
		return expectState, nil
	}

	st, err := newAPIConnectionFromNames(c, "noconfig", "admin@local", "admin", store, apiOpen, noBootstrapConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	// The addresses should have been updated.
	c.Assert(
		store.Controllers["noconfig"].APIEndpoints,
		jc.DeepEquals,
		[]string{"0.1.2.3:1234", "[2001:db8::1]:1234"},
	)

	controllerBefore, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)

	// If APIHostPorts haven't changed, then the store won't be updated.
	stubStore := jujuclienttesting.WrapClientStore(store)
	st, err = newAPIConnectionFromNames(c, "noconfig", "admin@local", "admin", stubStore, apiOpen, noBootstrapConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 2)
	stubStore.CheckCallNames(c, "AccountByName", "ModelByName", "ControllerByName")

	controllerAfter, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerBefore, gc.DeepEquals, controllerAfter)
}

func (s *NewAPIClientSuite) TestWithInfoError(c *gc.C) {
	store := newClientStore(c, "noconfig")
	err := store.UpdateController("noconfig", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
	})
	c.Assert(err, jc.ErrorIsNil)

	expectErr := fmt.Errorf("an error")
	getBootstrapConfig := func(string) (*config.Config, error) {
		return nil, expectErr
	}

	client, err := newAPIConnectionFromNames(c, "noconfig", "", "", store, panicAPIOpen, getBootstrapConfig)
	c.Assert(errors.Cause(err), gc.Equals, expectErr)
	c.Assert(client, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithInfoNoAddresses(c *gc.C) {
	store := newClientStore(c, "noconfig")
	err := store.UpdateController("noconfig", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
	})
	c.Assert(err, jc.ErrorIsNil)

	st, err := newAPIConnectionFromNames(c, "noconfig", "admin@local", "", store, panicAPIOpen, noBootstrapConfig)
	c.Assert(err, gc.ErrorMatches, "bootstrap config for controller noconfig not found")
	c.Assert(st, gc.IsNil)
}

type mockedStateFlags int

const (
	noFlags        mockedStateFlags = 0x0000
	mockedHostPort mockedStateFlags = 0x0001
	mockedModelTag mockedStateFlags = 0x0002
)

func mockedAPIState(flags mockedStateFlags) *mockAPIState {
	hasHostPort := flags&mockedHostPort == mockedHostPort
	hasModelTag := flags&mockedModelTag == mockedModelTag
	addr := ""

	apiHostPorts := [][]network.HostPort{}
	if hasHostPort {
		var apiAddrs []network.Address
		ipv4Address := network.NewAddress("0.1.2.3")
		ipv6Address := network.NewAddress("2001:db8::1")
		addr = net.JoinHostPort(ipv4Address.Value, "1234")
		apiAddrs = append(apiAddrs, ipv4Address, ipv6Address)
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
	c.Check(string(apiInfo.CACert), gc.Equals, "certificate")
	c.Check(apiInfo.Password, gc.Equals, "hunter2")
	c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
}

func (s *NewAPIClientSuite) TestWithInfoAPIOpenError(c *gc.C) {
	jujuClient := newClientStore(c, "noconfig")

	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		return nil, errors.Errorf("an error")
	}
	st, err := newAPIConnectionFromNames(c, "noconfig", "", "", jujuClient, apiOpen, noBootstrapConfig)
	// We expect to get the error from apiOpen, because it is not
	// fatal to have no bootstrap config.
	c.Assert(err, gc.ErrorMatches, "connecting with cached addresses: an error")
	c.Assert(st, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithSlowInfoConnect(c *gc.C) {
	c.Skip("wallyworld - this is a dumb test relying on an arbitary 50ms delay to pass")
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	_, store := s.bootstrapModel(c)
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
	st, err := newAPIConnectionFromNames(c,
		"local.my-controller", "admin@local", "only", store, apiOpen,
		modelcmd.NewGetBootstrapConfigFunc(store),
	)
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

func setEndpointAddressAndHostname(c *gc.C, store jujuclient.ControllerStore, addr, host string) {
	// Populate the controller details with known address and hostname.
	details, err := store.ControllerByName("local.my-controller")
	c.Assert(err, jc.ErrorIsNil)
	details.APIEndpoints = []string{addr}
	details.UnresolvedAPIEndpoints = []string{host}
	err = store.UpdateController("local.my-controller", *details)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NewAPIClientSuite) TestWithSlowConfigConnect(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	_, store := s.bootstrapModel(c)
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
		st, err := newAPIConnectionFromNames(c,
			"local.my-controller", "admin@local", "only", store, apiOpen,
			modelcmd.NewGetBootstrapConfigFunc(store),
		)
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
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env, store := s.bootstrapModel(c)
	setEndpointAddressAndHostname(c, store, "0.1.2.3", "infoapi.invalid")

	getBootstrapConfig := func(string) (*config.Config, error) {
		return env.Config(), nil
	}

	s.PatchValue(juju.ProviderConnectDelay, 0*time.Second)
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		if info.Addrs[0] == "infoapi.invalid" {
			return nil, fmt.Errorf("info connect failed")
		}
		return nil, fmt.Errorf("config connect failed")
	}
	st, err := newAPIConnectionFromNames(c, "local.my-controller", "admin@local", "only", store, apiOpen, getBootstrapConfig)
	c.Check(err, gc.ErrorMatches, "connecting with bootstrap config: config connect failed")
	c.Check(st, gc.IsNil)
}

// newClientStore returns a client store that contains information
// based on the given controller namd and info.
func newClientStore(c *gc.C, controllerName string) *jujuclienttesting.MemStore {
	store := jujuclienttesting.NewMemStore()
	err := store.UpdateController(controllerName, jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
		APIEndpoints:   []string{"foo.invalid"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = store.UpdateModel(controllerName, "admin@local", "admin", jujuclient.ModelDetails{
		fakeUUID,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Models belong to accounts, so we must have an account even
	// if "creds" is not initialised. If it is, it may overwrite
	// this one.
	err = store.UpdateAccount(controllerName, "admin@local", jujuclient.AccountDetails{
		User:     "admin@local",
		Password: "hunter2",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = store.SetCurrentAccount(controllerName, "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	return store
}

type CacheAPIEndpointsSuite struct {
	jujutesting.JujuConnSuite

	hostPorts   [][]network.HostPort
	modelTag    names.ModelTag
	apiHostPort network.HostPort

	resolveSeq      int
	resolveNumCalls int
	numResolved     int
	gocheckC        *gc.C
}

var _ = gc.Suite(&CacheAPIEndpointsSuite{})

func (s *CacheAPIEndpointsSuite) SetUpTest(c *gc.C) {
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

	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(juju.ResolveOrDropHostnames, s.mockResolveOrDropHostnames)

	apiHostPort, err := network.ParseHostPorts(s.APIState.Addr())
	c.Assert(err, jc.ErrorIsNil)
	s.apiHostPort = apiHostPort[0]
}

func (s *CacheAPIEndpointsSuite) assertCreateController(c *gc.C, name string) jujuclient.ControllerDetails {
	// write controller
	controllerDetails := jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
	}
	err := s.ControllerStore.UpdateController(name, controllerDetails)
	c.Assert(err, jc.ErrorIsNil)
	return controllerDetails
}

func (s *CacheAPIEndpointsSuite) assertControllerDetailsUpdated(c *gc.C, name string, check gc.Checker) {
	found, err := s.ControllerStore.ControllerByName(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.UnresolvedAPIEndpoints, check, 0)
	c.Assert(found.APIEndpoints, check, 0)
}

func (s *CacheAPIEndpointsSuite) assertControllerUpdated(c *gc.C, name string) {
	s.assertControllerDetailsUpdated(c, name, gc.Not(gc.HasLen))
}

func (s *CacheAPIEndpointsSuite) assertControllerNotUpdated(c *gc.C, name string) {
	s.assertControllerDetailsUpdated(c, name, gc.HasLen)
}

func (s *CacheAPIEndpointsSuite) TestPrepareEndpointsForCaching(c *gc.C) {
	s.assertCreateController(c, "controller-name1")
	err := juju.UpdateControllerAddresses(s.ControllerStore, "controller-name1", s.hostPorts, s.apiHostPort)
	c.Assert(err, jc.ErrorIsNil)
	controllerDetails, err := s.ControllerStore.ControllerByName("controller-name1")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpoints(c, controllerDetails)
	s.assertControllerUpdated(c, "controller-name1")
}

func (s *CacheAPIEndpointsSuite) TestResolveSkippedWhenHostnamesUnchanged(c *gc.C) {
	// Test that if new endpoints hostnames are the same as the
	// cached, no DNS resolution happens (i.e. we don't resolve on
	// every connection, but as needed).
	hps := network.NewHostPorts(1234,
		"8.8.8.8",
		"example.com",
		"10.0.0.1",
	)
	controllerDetails := jujuclient.ControllerDetails{
		ControllerUUID:         fakeUUID,
		CACert:                 "certificate",
		UnresolvedAPIEndpoints: network.HostPortsToStrings(hps),
	}
	err := s.ControllerStore.UpdateController("controller-name", controllerDetails)
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		controllerDetails, [][]network.HostPort{hps},
	)
	c.Assert(addrs, gc.IsNil)
	c.Assert(hosts, gc.IsNil)
	c.Assert(changed, jc.IsFalse)
	c.Assert(s.resolveNumCalls, gc.Equals, 0)
	c.Assert(
		c.GetTestLog(),
		jc.Contains,
		"DEBUG juju.juju API hostnames unchanged - not resolving",
	)
}

func (s *CacheAPIEndpointsSuite) TestResolveCalledWithChangedHostnames(c *gc.C) {
	// Test that if new endpoints hostnames are different than the
	// cached hostnames DNS resolution happens and we compare resolved
	// addresses.
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
	controllerDetails := jujuclient.ControllerDetails{
		ControllerUUID:         fakeUUID,
		CACert:                 "certificate",
		UnresolvedAPIEndpoints: strUnsorted,
	}
	err := s.ControllerStore.UpdateController("controller-name", controllerDetails)
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		controllerDetails, [][]network.HostPort{unsortedHPs},
	)
	c.Assert(addrs, jc.DeepEquals, strResolved)
	c.Assert(hosts, jc.DeepEquals, strSorted)
	c.Assert(changed, jc.IsTrue)
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 2)
	expectLog := fmt.Sprintf("DEBUG juju.juju API hostnames changed from %v to %v - resolving hostnames", unsortedHPs, sortedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
	expectLog = fmt.Sprintf("INFO juju.juju new API addresses to cache %v", resolvedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
}

func (s *CacheAPIEndpointsSuite) TestAfterResolvingUnchangedAddressesNotCached(c *gc.C) {
	// Test that if new endpoints hostnames are different than the
	// cached hostnames, but after resolving the addresses match the
	// cached addresses, the cache is not changed.

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
	controllerDetails := jujuclient.ControllerDetails{
		ControllerUUID:         fakeUUID,
		CACert:                 "certificate",
		UnresolvedAPIEndpoints: strUnsorted,
		APIEndpoints:           strResolved,
	}
	err := s.ControllerStore.UpdateController("controller-name", controllerDetails)
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		controllerDetails, [][]network.HostPort{unsortedHPs},
	)
	c.Assert(addrs, gc.IsNil)
	c.Assert(hosts, gc.IsNil)
	c.Assert(changed, jc.IsFalse)
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 2)
	expectLog := fmt.Sprintf("DEBUG juju.juju API hostnames changed from %v to %v - resolving hostnames", unsortedHPs, sortedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
	expectLog = "DEBUG juju.juju API addresses unchanged"
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
}

func (s *CacheAPIEndpointsSuite) TestResolveCalledWithInitialEndpoints(c *gc.C) {
	// Test that if no hostnames exist cached we call resolve (i.e.
	// simulate the behavior right after bootstrap)

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

	controllerDetails := jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
	}
	err := s.ControllerStore.UpdateController("controller-name", controllerDetails)
	c.Assert(err, jc.ErrorIsNil)

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(
		controllerDetails, [][]network.HostPort{unsortedHPs},
	)
	c.Assert(addrs, jc.DeepEquals, strResolved)
	c.Assert(hosts, jc.DeepEquals, strSorted)
	c.Assert(changed, jc.IsTrue)
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 2)
	expectLog := fmt.Sprintf("DEBUG juju.juju API hostnames %v - resolving hostnames", sortedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
	expectLog = fmt.Sprintf("INFO juju.juju new API addresses to cache %v", resolvedHPs)
	c.Assert(c.GetTestLog(), jc.Contains, expectLog)
}

func (s *CacheAPIEndpointsSuite) assertEndpoints(c *gc.C, controllerDetails *jujuclient.ControllerDetails) {
	c.Assert(s.resolveNumCalls, gc.Equals, 1)
	c.Assert(s.numResolved, gc.Equals, 10)
	// Check Addresses after resolving.
	c.Check(controllerDetails.APIEndpoints, jc.DeepEquals, []string{
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
	c.Check(controllerDetails.UnresolvedAPIEndpoints, jc.DeepEquals, []string{
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

func noBootstrapConfig(controllerName string) (*config.Config, error) {
	return nil, errors.NotFoundf("bootstrap config for controller %s", controllerName)
}

func newAPIConnectionFromNames(
	c *gc.C,
	controller, account, model string,
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
	getBootstrapConfig func(string) (*config.Config, error),
) (api.Connection, error) {
	params := juju.NewAPIConnectionParams{
		Store:           store,
		ControllerName:  controller,
		BootstrapConfig: getBootstrapConfig,
		DialOpts:        api.DefaultDialOpts(),
	}
	if account != "" {
		accountDetails, err := store.AccountByName(controller, account)
		c.Assert(err, jc.ErrorIsNil)
		params.AccountDetails = accountDetails
	}
	if model != "" {
		modelDetails, err := store.ModelByName(controller, account, model)
		c.Assert(err, jc.ErrorIsNil)
		params.ModelUUID = modelDetails.ModelUUID
	}
	return juju.NewAPIFromStore(params, apiOpen)
}
