// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/jsoncodec"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type NewAPIClientSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testing.MgoSuite
	envtesting.ToolsFixture
}

var fakeUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

var _ = gc.Suite(&NewAPIClientSuite{})

func (s *NewAPIClientSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *NewAPIClientSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (s *NewAPIClientSuite) SetUpTest(c *gc.C) {
	s.ToolsFixture.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.PatchValue(&dummy.LogDir, c.MkDir())
}

func (s *NewAPIClientSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)
	s.ToolsFixture.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
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

	st, err := newAPIConnectionFromNames(c, "noconfig", "admin/admin", store, apiOpen)
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
	st, err = newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
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
	st, err := newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.Equals, expectState)
	c.Assert(called, gc.Equals, 1)
	stubStore.CheckCallNames(c, "AccountDetails", "ModelByName", "ControllerByName", "UpdateController", "AccountDetails", "UpdateAccount")

	c.Assert(
		store.Accounts["noconfig"],
		jc.DeepEquals,
		jujuclient.AccountDetails{User: "admin", Password: "hunter2", LastKnownAccess: "superuser"},
	)
}

func (s *NewAPIClientSuite) TestUpdatesPublicDNSName(c *gc.C) {
	apiOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		conn := mockedAPIState(noFlags)
		conn.publicDNSName = "somewhere.invalid"
		conn.addr = "0.1.2.3:1234"
		return conn, nil
	}

	store := newClientStore(c, "controllername")
	_, err := newAPIConnectionFromNames(c, "controllername", "", store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store.Controllers["controllername"].PublicDNSName, gc.Equals, "somewhere.invalid")
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

	redirHPs := []network.MachineHostPorts{{
		network.MachineHostPort{MachineAddress: network.NewMachineAddress("0.0.9.9"), NetPort: network.NetPort(1234)},
		network.MachineHostPort{MachineAddress: network.NewMachineAddress("0.0.9.10"), NetPort: network.NetPort(1235)},
	}}

	openCount := 0
	redirOpen := func(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		c.Check(apiInfo.ModelTag.Id(), gc.Equals, fakeUUID)
		openCount++
		switch openCount {
		case 1:
			c.Check(apiInfo.Addrs, jc.DeepEquals, []string{"0.1.2.3:5678"})
			c.Check(apiInfo.CACert, gc.Equals, "certificate")
			return nil, errors.Trace(&api.RedirectError{
				Servers:        redirHPs,
				CACert:         "alternative CA cert",
				FollowRedirect: true,
			})
		case 2:
			c.Check(apiInfo.Addrs, jc.DeepEquals, network.CollapseToHostPorts(redirHPs).Strings())
			c.Check(apiInfo.CACert, gc.Equals, "alternative CA cert")
			st := mockedAPIState(noFlags)
			st.apiHostPorts = redirHPs

			st.modelTag = fakeUUID
			return st, nil
		}
		c.Errorf("OpenAPI called too many times")
		return nil, fmt.Errorf("OpenAPI called too many times")
	}

	st0, err := newAPIConnectionFromNames(c, "ctl", "admin/admin", store, redirOpen)
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

func (s *NewAPIClientSuite) TestDialedAddressIsCached(c *gc.C) {
	store := jujuclient.NewMemStore()
	err := store.AddController("foo", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		APIEndpoints: []string{
			"example1:1111",
			"example2:2222",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	dialed := make(chan string, 10)
	start := make(chan struct{})
	// Wait for both dials to complete, so we
	// know their addresses are cached.
	go func() {
		addrs := make(map[string]bool)
		for len(addrs) < 2 {
			addrs[<-dialed] = true
		}
		// Allow the dials to complete.
		close(start)
	}()
	conn, err := juju.NewAPIConnection(juju.NewAPIConnectionParams{
		Store:          store,
		ControllerName: "foo",
		DialOpts: api.DialOpts{
			DialWebsocket: func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
				apiConn := testRootAPI{
					serverAddrs: params.FromProviderHostsPorts([]network.ProviderHostPorts{{
						network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("example3"), NetPort: 3333},
						network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("example4"), NetPort: 4444},
					}}),
				}
				dialed <- ipAddr
				<-start
				if ipAddr != "0.1.1.2:1111" {
					return nil, errors.New("fail")
				}
				return jsoncodec.NetJSONConn(apitesting.FakeAPIServer(apiConn)), nil
			},
			IPAddrResolver: apitesting.IPAddrResolverMap{
				"example1": {"0.1.1.1", "0.1.1.2"},
				"example2": {"0.2.2.2"},
			},
		},
		AccountDetails: new(jujuclient.AccountDetails),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	details, err := store.ControllerByName("foo")
	c.Assert(err, jc.ErrorIsNil)
	// The cache should contain both results. The IP address
	// that was successfully dialed should be at the start of its
	// slice.
	c.Assert(details.DNSCache, jc.DeepEquals, map[string][]string{
		"example1": {"0.1.1.2", "0.1.1.1"},
		"example2": {"0.2.2.2"},
	})
	// The API addresses should have all the returned server addresses
	// there as well as the one we actually succeeded in dialing.
	// The successfully dialed address should be at the start.
	c.Assert(details.APIEndpoints, jc.DeepEquals, []string{
		"example1:1111",
		"example3:3333",
		"example4:4444",
	})
}

func (s *NewAPIClientSuite) TestWithExistingDNSCache(c *gc.C) {
	store := jujuclient.NewMemStore()
	err := store.AddController("foo", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		APIEndpoints: []string{
			"example1:1111",
			"example3:3333",
			"example4:4444",
		},
		DNSCache: map[string][]string{
			"example1": {"0.1.1.2", "0.1.1.1"},
			"example2": {"0.2.2.2"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	start := make(chan struct{})
	conn, err := juju.NewAPIConnection(juju.NewAPIConnectionParams{
		Store:          store,
		ControllerName: "foo",
		DialOpts: api.DialOpts{
			DialWebsocket: func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
				apiConn := testRootAPI{
					serverAddrs: params.FromProviderHostsPorts([]network.ProviderHostPorts{{
						network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("example3"), NetPort: 3333},
						network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("example5"), NetPort: 5555},
					}}),
				}
				c.Logf("Dial: %q requested", ipAddr)
				if ipAddr != "0.1.1.2:1111" {
					// It's not the blessed IP address - block indefinitely
					// until we're called upon to start.
					select {
					case <-start:
					case <-time.After(testing.LongWait):
						c.Fatalf("timeout while waiting for start dialing %v", ipAddr)
					}
					return nil, errors.New("fail")
				}
				// We're trying to connect to the blessed IP address.
				// Succeed immediately.
				return jsoncodec.NetJSONConn(apitesting.FakeAPIServer(apiConn)), nil
			},
			IPAddrResolver: ipAddrResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
				c.Logf("Resolve: %q requested", host)
				// We shouldn't block here, because IP Address lookups are done blocking in the main loop.
				return nil, errors.New("no DNS available")
			}),
		},
		AccountDetails: new(jujuclient.AccountDetails),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	close(start)
	details, err := store.ControllerByName("foo")
	c.Assert(err, jc.ErrorIsNil)
	// The DNS cache should not have changed.
	c.Assert(details.DNSCache, jc.DeepEquals, map[string][]string{
		"example1": {"0.1.1.2", "0.1.1.1"},
		"example2": {"0.2.2.2"},
	})
	// The API addresses should have all the returned server addresses
	// there as well as the one we actually succeeded in dialing.
	// The successfully dialed address should be still at the start.
	c.Assert(details.APIEndpoints, jc.DeepEquals, []string{
		"example1:1111",
		"example3:3333",
		"example5:5555",
	})
}

func (s *NewAPIClientSuite) TestEndpointFiltering(c *gc.C) {
	store := jujuclient.NewMemStore()
	err := store.AddController("foo", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		APIEndpoints: []string{
			"example1:1111",
		},
		DNSCache: map[string][]string{
			"example1": {"0.1.1.1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	serverAddrs := params.FromProviderHostsPorts([]network.ProviderHostPorts{{
		network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("0.1.2.3"), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("2001:db8::1"), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("10.0.0.1"), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("127.0.0.1"), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("169.254.1.1"), NetPort: 1234},
		//Duplicate
		network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("0.1.2.3"), NetPort: 1234},
		//Duplicate host, same IP.
		network.ProviderHostPort{ProviderAddress: network.NewProviderAddress("0.1.2.3"), NetPort: 1235},
	}})

	conn, err := juju.NewAPIConnection(juju.NewAPIConnectionParams{
		Store:          store,
		ControllerName: "foo",
		DialOpts: api.DialOpts{
			DialWebsocket: func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
				apiConn := testRootAPI{
					serverAddrs: serverAddrs,
				}
				return jsoncodec.NetJSONConn(apitesting.FakeAPIServer(apiConn)), nil
			},
			IPAddrResolver: ipAddrResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
				return nil, errors.New("no DNS available")
			}),
		},
		AccountDetails: new(jujuclient.AccountDetails),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	details, err := store.ControllerByName("foo")
	c.Assert(err, jc.ErrorIsNil)
	// The API addresses should have filtered out duplicates
	// and unusable addresses.
	c.Assert(details.APIEndpoints, jc.DeepEquals, []string{
		"example1:1111",
		"0.1.2.3:1234",
		"[2001:db8::1]:1234",
		"10.0.0.1:1234",
		"0.1.2.3:1235",
	})
}

var moveToFrontTests = []struct {
	item   string
	items  []string
	expect []string
}{{
	item:   "x",
	items:  []string{"y", "x"},
	expect: []string{"x", "y"},
}, {
	item:   "z",
	items:  []string{"y", "x"},
	expect: []string{"y", "x"},
}, {
	item:   "y",
	items:  []string{"y", "x"},
	expect: []string{"y", "x"},
}, {
	item:   "x",
	items:  []string{"y", "x", "z"},
	expect: []string{"x", "y", "z"},
}, {
	item:   "d",
	items:  []string{"a", "b", "c", "d", "e", "f"},
	expect: []string{"d", "a", "b", "c", "e", "f"},
}}

func (s *NewAPIClientSuite) TestMoveToFront(c *gc.C) {
	for i, test := range moveToFrontTests {
		c.Logf("test %d: moveToFront %q %v", i, test.item, test.items)
		juju.MoveToFront(test.item, test.items)
		c.Assert(test.items, jc.DeepEquals, test.expect)
	}
}

type testRootAPI struct {
	serverAddrs [][]params.HostPort
}

func (r testRootAPI) Admin(id string) (testAdminAPI, error) {
	return testAdminAPI{r: r}, nil
}

type testAdminAPI struct {
	r testRootAPI
}

func (a testAdminAPI) Login(req params.LoginRequest) params.LoginResult {
	return params.LoginResult{
		ControllerTag: names.NewControllerTag(fakeUUID).String(),
		Servers:       a.r.serverAddrs,
		ServerVersion: version.Current.String(),
	}
}

func checkCommonAPIInfoAttrs(c *gc.C, apiInfo *api.Info, opts api.DialOpts) {
	opts.DNSCache = nil
	c.Check(apiInfo.Tag, gc.Equals, names.NewUserTag("admin"))
	c.Check(apiInfo.CACert, gc.Equals, "certificate")
	c.Check(apiInfo.Password, gc.Equals, "hunter2")
	c.Check(opts, gc.DeepEquals, api.DefaultDialOpts())
}

// newClientStore returns a client store that contains information
// based on the given controller name and info.
func newClientStore(c *gc.C, controllerName string) *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	err := store.AddController(controllerName, jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
		APIEndpoints:   []string{"0.1.2.3:5678"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = store.UpdateModel(controllerName, "admin/admin", jujuclient.ModelDetails{
		ModelUUID: fakeUUID, ModelType: model.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Models belong to accounts, so we must have an account even
	// if "creds" is not initialised. If it is, it may overwrite
	// this one.
	err = store.UpdateAccount(controllerName, jujuclient.AccountDetails{
		User:     "admin",
		Password: "hunter2",
	})
	c.Assert(err, jc.ErrorIsNil)
	return store
}

func newAPIConnectionFromNames(
	c *gc.C,
	controller, model string,
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
) (api.Connection, error) {
	args := juju.NewAPIConnectionParams{
		Store:          store,
		ControllerName: controller,
		DialOpts:       api.DefaultDialOpts(),
		OpenAPI:        apiOpen,
	}
	accountDetails, err := store.AccountDetails(controller)
	if !errors.IsNotFound(err) {
		c.Assert(err, jc.ErrorIsNil)
		args.AccountDetails = accountDetails
	}
	if model != "" {
		modelDetails, err := store.ModelByName(controller, model)
		c.Assert(err, jc.ErrorIsNil)
		args.ModelUUID = modelDetails.ModelUUID
	}
	return juju.NewAPIConnection(args)
}

type ipAddrResolverFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

func (f ipAddrResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}
