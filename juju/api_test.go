// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/filestorage"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
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
	cs.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
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
	dummy.Reset(c)
	cs.ToolsFixture.TearDownTest(c)
	cs.MgoSuite.TearDownTest(c)
	cs.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *NewAPIClientSuite) bootstrapModel(c *gc.C) (environs.Environ, jujuclient.ClientStore) {
	const controllerName = "my-controller"

	store := jujuclienttesting.NewMemStore()

	ctx := envtesting.BootstrapContext(c)

	env, err := bootstrap.Prepare(ctx, store, bootstrap.PrepareParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		ControllerName:   controllerName,
		ModelConfig:      dummy.SampleConfig(),
		Cloud:            dummy.SampleCloudSpec(),
		AdminSecret:      "admin-secret",
	})
	c.Assert(err, jc.ErrorIsNil)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	err = bootstrap.Bootstrap(ctx, env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		CloudName:        "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		AdminSecret:  "admin-secret",
		CAPrivateKey: coretesting.CAKey,
	})
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

	st, err := newAPIConnectionFromNames(c, "noconfig", "admin@local/admin", store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	// The addresses should have been updated.
	c.Assert(
		store.Controllers["noconfig"].APIEndpoints,
		jc.DeepEquals,
		[]string{"0.1.2.3:1234", "[2001:db8::1]:1234"},
	)
	c.Assert(
		store.Controllers["noconfig"].AgentVersion,
		gc.Equals,
		"1.2.3",
	)

	controllerBefore, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)

	// If APIHostPorts or agent version haven't changed, then the store won't be updated.
	stubStore := jujuclienttesting.WrapClientStore(store)
	st, err = newAPIConnectionFromNames(c, "noconfig", "admin@local/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 2)
	stubStore.CheckCallNames(c, "AccountDetails", "ModelByName", "ControllerByName", "AccountDetails", "UpdateAccount")

	controllerAfter, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerBefore, gc.DeepEquals, controllerAfter)
}

func (s *NewAPIClientSuite) TestUpdatesLastKnownAccess(c *gc.C) {
	store := newClientStore(c, "noconfig")

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.ModelTag, gc.Equals, names.NewModelTag(fakeUUID))
		called++
		return expectState, nil
	}

	stubStore := jujuclienttesting.WrapClientStore(store)
	st, err := newAPIConnectionFromNames(c, "noconfig", "admin@local/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	stubStore.CheckCallNames(c, "AccountDetails", "ModelByName", "ControllerByName", "UpdateController", "AccountDetails", "UpdateAccount")

	c.Assert(
		store.Accounts["noconfig"],
		jc.DeepEquals,
		jujuclient.AccountDetails{User: "admin@local", Password: "hunter2", LastKnownAccess: "superuser"},
	)
}

func (s *NewAPIClientSuite) TestWithInfoNoAddresses(c *gc.C) {
	store := newClientStore(c, "noconfig")
	err := store.UpdateController("noconfig", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
	})
	c.Assert(err, jc.ErrorIsNil)

	st, err := newAPIConnectionFromNames(c, "noconfig", "", store, panicAPIOpen)
	c.Assert(err, gc.ErrorMatches, "no API addresses")
	c.Assert(st, gc.IsNil)
}

func (s *NewAPIClientSuite) TestWithRedirect(c *gc.C) {
	store := newClientStore(c, "ctl")
	err := store.UpdateController("ctl", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
		APIEndpoints:   []string{"0.1.2.3:5678"},
	})
	c.Assert(err, jc.ErrorIsNil)

	controllerBefore, err := store.ControllerByName("ctl")
	c.Assert(err, jc.ErrorIsNil)

	redirHPs := []string{"0.0.9.9:1234", "0.0.9.10:1235"}
	openCount := 0
	redirOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		c.Check(apiInfo.ModelTag.Id(), gc.Equals, fakeUUID)
		openCount++
		switch openCount {
		case 1:
			c.Check(apiInfo.Addrs, jc.DeepEquals, []string{"0.1.2.3:5678"})
			c.Check(apiInfo.CACert, gc.Equals, "certificate")
			return nil, errors.Trace(&api.RedirectError{
				Servers: [][]network.HostPort{mustParseHostPorts(redirHPs)},
				CACert:  "alternative CA cert",
			})
		case 2:
			c.Check(apiInfo.Addrs, jc.DeepEquals, []string{"0.0.9.9:1234", "0.0.9.10:1235"})
			c.Check(apiInfo.CACert, gc.Equals, "alternative CA cert")
			st := mockedAPIState(noFlags)
			st.apiHostPorts = [][]network.HostPort{mustParseHostPorts(redirHPs)}
			st.modelTag = fakeUUID
			return st, nil
		}
		c.Errorf("OpenAPI called too many times")
		return nil, fmt.Errorf("OpenAPI called too many times")
	}

	st0, err := newAPIConnectionFromNames(c, "ctl", "admin@local/admin", store, redirOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(openCount, gc.Equals, 2)
	st := st0.(*mockAPIState)
	c.Assert(st.modelTag, gc.Equals, fakeUUID)

	// Check that the addresses of the original controller
	// have not been changed.
	controllerAfter, err := store.ControllerByName("ctl")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerBefore, gc.DeepEquals, controllerAfter)
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
	st, err := newAPIConnectionFromNames(c, "noconfig", "", jujuClient, apiOpen)
	// We expect to get the error from apiOpen, because it is not
	// fatal to have no bootstrap config.
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(st, gc.IsNil)
}

// newClientStore returns a client store that contains information
// based on the given controller name and info.
func newClientStore(c *gc.C, controllerName string) *jujuclienttesting.MemStore {
	store := jujuclienttesting.NewMemStore()
	err := store.AddController(controllerName, jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
		APIEndpoints:   []string{"0.1.2.3:5678"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = store.UpdateModel(controllerName, "admin@local/admin", jujuclient.ModelDetails{
		fakeUUID,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Models belong to accounts, so we must have an account even
	// if "creds" is not initialised. If it is, it may overwrite
	// this one.
	err = store.UpdateAccount(controllerName, jujuclient.AccountDetails{
		User:     "admin@local",
		Password: "hunter2",
	})
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
	err := s.ControllerStore.AddController(name, controllerDetails)
	c.Assert(err, jc.ErrorIsNil)
	return controllerDetails
}

func (s *CacheAPIEndpointsSuite) assertControllerDetailsUpdated(c *gc.C, name string, check gc.Checker) {
	found, err := s.ControllerStore.ControllerByName(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.UnresolvedAPIEndpoints, check, 0)
	c.Assert(found.APIEndpoints, check, 0)
	c.Assert(found.AgentVersion, gc.Equals, "1.2.3")
	c.Assert(found.ModelCount, gc.IsNil)
	c.Assert(found.MachineCount, gc.IsNil)
	c.Assert(found.ControllerMachineCount, gc.Equals, 0)
}

func (s *CacheAPIEndpointsSuite) assertControllerUpdated(c *gc.C, name string) {
	s.assertControllerDetailsUpdated(c, name, gc.Not(gc.HasLen))
}

func (s *CacheAPIEndpointsSuite) assertControllerNotUpdated(c *gc.C, name string) {
	s.assertControllerDetailsUpdated(c, name, gc.HasLen)
}

func (s *CacheAPIEndpointsSuite) TestPrepareEndpointsForCaching(c *gc.C) {
	s.assertCreateController(c, "controller-name1")
	params := juju.UpdateControllerParams{
		AgentVersion:     "1.2.3",
		AddrConnectedTo:  []network.HostPort{s.apiHostPort},
		CurrentHostPorts: s.hostPorts,
	}
	err := juju.UpdateControllerDetailsFromLogin(s.ControllerStore, "controller-name1", params)
	c.Assert(err, jc.ErrorIsNil)
	controllerDetails, err := s.ControllerStore.ControllerByName("controller-name1")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEndpoints(c, controllerDetails)
	s.assertControllerUpdated(c, "controller-name1")
}

func intptr(i int) *int {
	return &i
}

func (s *CacheAPIEndpointsSuite) TestUpdateModelMachineCount(c *gc.C) {
	s.assertCreateController(c, "controller-name1")
	params := juju.UpdateControllerParams{
		AgentVersion:           "1.2.3",
		ControllerMachineCount: intptr(1),
		ModelCount:             intptr(2),
		MachineCount:           intptr(3),
	}
	err := juju.UpdateControllerDetailsFromLogin(s.ControllerStore, "controller-name1", params)
	c.Assert(err, jc.ErrorIsNil)
	controllerDetails, err := s.ControllerStore.ControllerByName("controller-name1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerDetails.UnresolvedAPIEndpoints, gc.HasLen, 0)
	c.Assert(controllerDetails.APIEndpoints, gc.HasLen, 0)
	c.Assert(controllerDetails.AgentVersion, gc.Equals, "1.2.3")
	c.Assert(controllerDetails.ControllerMachineCount, gc.Equals, 1)
	c.Assert(*controllerDetails.ModelCount, gc.Equals, 2)
	c.Assert(*controllerDetails.MachineCount, gc.Equals, 3)
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
	err := s.ControllerStore.AddController("controller-name", controllerDetails)
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
	err := s.ControllerStore.AddController("controller-name", controllerDetails)
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
	err := s.ControllerStore.AddController("controller-name", controllerDetails)
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
	err := s.ControllerStore.AddController("controller-name", controllerDetails)
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

func newAPIConnectionFromNames(
	c *gc.C,
	controller, model string,
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
) (api.Connection, error) {
	params := juju.NewAPIConnectionParams{
		Store:          store,
		ControllerName: controller,
		DialOpts:       api.DefaultDialOpts(),
		OpenAPI:        apiOpen,
	}
	accountDetails, err := store.AccountDetails(controller)
	if !errors.IsNotFound(err) {
		c.Assert(err, jc.ErrorIsNil)
		params.AccountDetails = accountDetails
	}
	if model != "" {
		modelDetails, err := store.ModelByName(controller, model)
		c.Assert(err, jc.ErrorIsNil)
		params.ModelUUID = modelDetails.ModelUUID
	}
	return juju.NewAPIConnection(params)
}

func mustParseHostPorts(ss []string) []network.HostPort {
	hps, err := network.ParseHostPorts(ss...)
	if err != nil {
		panic(err)
	}
	return hps
}
