// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/version"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/rpc/params"
)

type NewAPIClientSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

var fakeUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

var _ = tc.Suite(&NewAPIClientSuite{})

func (s *NewAPIClientSuite) SetUpSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *NewAPIClientSuite) TearDownSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (s *NewAPIClientSuite) SetUpTest(c *tc.C) {
	s.ToolsFixture.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *NewAPIClientSuite) TearDownTest(c *tc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *NewAPIClientSuite) TestWithBootstrapConfig(c *tc.C) {
	store := newClientStore(c, "noconfig")

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		c.Check(apiInfo.ModelTag, tc.Equals, names.NewModelTag(fakeUUID))
		called++
		return expectState, nil
	}

	st, err := newAPIConnectionFromNames(c, "noconfig", "admin/admin", store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, tc.Equals, expectState)
	c.Assert(called, tc.Equals, 1)
	// The addresses should have been updated.
	c.Assert(
		store.Controllers["noconfig"].APIEndpoints,
		jc.DeepEquals,
		[]string{"0.1.2.3:1234", "[2001:db8::1]:1234"},
	)
	c.Assert(
		store.Controllers["noconfig"].AgentVersion,
		tc.Equals,
		"1.2.3",
	)

	controllerBefore, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)

	// If APIHostPorts or agent version haven't changed, then the store won't be updated.
	stubStore := jujuclienttesting.WrapClientStore(store)
	st, err = newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, tc.Equals, expectState)
	c.Assert(called, tc.Equals, 2)
	stubStore.CheckCallNames(c, "AccountDetails", "ModelByName", "ControllerByName")

	controllerAfter, err := store.ControllerByName("noconfig")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerBefore, tc.DeepEquals, controllerAfter)
}

func (s *NewAPIClientSuite) TestIncorrectAuthTag(c *tc.C) {
	store := newClientStore(c, "noconfig")
	err := store.UpdateAccount("noconfig", jujuclient.AccountDetails{
		User: "wally@external",
	})
	c.Assert(err, jc.ErrorIsNil)

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		called++
		expectState.authTag = names.NewUserTag("simon@external")
		return expectState, nil
	}

	stubStore := jujuclienttesting.WrapClientStore(store)
	_, err = newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIs, errors.Unauthorized)
}

func (s *NewAPIClientSuite) TestCorrectAuthTag(c *tc.C) {
	store := newClientStore(c, "noconfig")
	err := store.UpdateAccount("noconfig", jujuclient.AccountDetails{
		User: "wally@external",
	})
	c.Assert(err, jc.ErrorIsNil)

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		called++
		expectState.authTag = names.NewUserTag("wally@external")
		return expectState, nil
	}

	stubStore := jujuclienttesting.WrapClientStore(store)
	_, err = newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
}

// TestEmptyStoreAuthTag simulates when the user issues an unqualified
// "juju login" and is redirected to an external identity provider.
// It is the provider that providers the identity, not the args.
func (s *NewAPIClientSuite) TestEmptyStoreAuthTag(c *tc.C) {
	store := newClientStore(c, "noconfig")
	err := store.RemoveAccount("noconfig")
	c.Assert(err, jc.ErrorIsNil)

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		called++
		expectState.authTag = names.NewUserTag("wally@external")
		return expectState, nil
	}

	stubStore := jujuclienttesting.WrapClientStore(store)
	_, err = newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NewAPIClientSuite) TestIncorrectAdminAuthTag(c *tc.C) {
	store := newClientStore(c, "noconfig")

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		called++
		expectState.authTag = names.NewUserTag("wally@external")
		return expectState, nil
	}

	stubStore := jujuclienttesting.WrapClientStore(store)
	_, err := newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIs, errors.Unauthorized)
}

func (s *NewAPIClientSuite) TestCorrectAdminAuthTag(c *tc.C) {
	store := newClientStore(c, "noconfig")

	called := 0
	expectState := mockedAPIState(mockedHostPort | mockedModelTag)
	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		checkCommonAPIInfoAttrs(c, apiInfo, opts)
		called++
		return expectState, nil
	}

	stubStore := jujuclienttesting.WrapClientStore(store)
	_, err := newAPIConnectionFromNames(c, "noconfig", "admin/admin", stubStore, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NewAPIClientSuite) TestUpdatesPublicDNSName(c *tc.C) {
	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		c.Assert(apiInfo.ControllerUUID, tc.Equals, fakeUUID)
		conn := mockedAPIState(noFlags)
		conn.publicDNSName = "somewhere.invalid"
		conn.addr = &url.URL{Scheme: "wss", Host: "0.1.2.3:1234"}
		return conn, nil
	}

	store := newClientStore(c, "controllername")
	_, err := newAPIConnectionFromNames(c, "controllername", "", store, apiOpen)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store.Controllers["controllername"].PublicDNSName, tc.Equals, "somewhere.invalid")
}

func (s *NewAPIClientSuite) TestWithInfoNoAddresses(c *tc.C) {
	store := newClientStore(c, "noconfig")
	err := store.UpdateController("noconfig", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		CACert:         "certificate",
	})
	c.Assert(err, jc.ErrorIsNil)

	st, err := newAPIConnectionFromNames(c, "noconfig", "", store, panicAPIOpen)
	c.Assert(err, jc.Satisfies, juju.IsNoAddressesError)
	c.Assert(st, tc.IsNil)
}

func (s *NewAPIClientSuite) TestWithMacaroons(c *tc.C) {
	store := newClientStore(c, "withmac")
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	err = store.UpdateAccount("withmac", jujuclient.AccountDetails{
		User:      "admin",
		Password:  "",
		Macaroons: []macaroon.Slice{{mac}},
	})
	c.Assert(err, jc.ErrorIsNil)
	ad, err := store.AccountDetails("withmac")
	c.Assert(err, jc.ErrorIsNil)
	info, _, err := juju.ConnectionInfo(juju.NewAPIConnectionParams{
		ControllerName:  "withmac",
		ControllerStore: store,
		AccountDetails:  ad,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Macaroons, tc.DeepEquals, []macaroon.Slice{{mac}})
}

func (s *NewAPIClientSuite) TestWithAddressOverride(c *tc.C) {
	store := newClientStore(c, "controllername")
	ad, err := store.AccountDetails("controllername")
	c.Assert(err, jc.ErrorIsNil)

	info, _, err := juju.ConnectionInfo(juju.NewAPIConnectionParams{
		ControllerName:  "controllername",
		ControllerStore: store,
		AccountDetails:  ad,
		APIEndpoints:    []string{"address-override"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Addrs, tc.DeepEquals, []string{"address-override"})
}

func (s *NewAPIClientSuite) TestWithRedirect(c *tc.C) {
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
	redirOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		c.Check(apiInfo.ControllerUUID, tc.Equals, fakeUUID)
		c.Check(apiInfo.ModelTag.Id(), tc.Equals, fakeUUID)
		openCount++
		switch openCount {
		case 1:
			c.Check(apiInfo.Addrs, jc.DeepEquals, []string{"0.1.2.3:5678"})
			c.Check(apiInfo.CACert, tc.Equals, "certificate")
			return nil, errors.Trace(&api.RedirectError{
				Servers:        redirHPs,
				CACert:         "alternative CA cert",
				FollowRedirect: true,
			})
		case 2:
			c.Check(apiInfo.Addrs, jc.DeepEquals, network.CollapseToHostPorts(redirHPs).Strings())
			c.Check(apiInfo.CACert, tc.Equals, "alternative CA cert")
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
	c.Assert(openCount, tc.Equals, 2)
	st := st0.(*mockAPIConnection)
	c.Assert(st.modelTag, tc.Equals, fakeUUID)

	// Check that the addresses of the original controller
	// have not been changed.
	controllerAfter, err := store.ControllerByName("ctl")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerBefore, tc.DeepEquals, controllerAfter)
}

func (s *NewAPIClientSuite) TestWithInfoAPIOpenError(c *tc.C) {
	jujuClient := newClientStore(c, "noconfig")

	apiOpen := func(ctx context.Context, apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
		return nil, errors.Errorf("an error")
	}
	st, err := newAPIConnectionFromNames(c, "noconfig", "", jujuClient, apiOpen)
	// We expect to get the error from apiOpen, because it is not
	// fatal to have no bootstrap config.
	c.Assert(err, tc.ErrorMatches, "an error")
	c.Assert(st, tc.IsNil)
}

func (s *NewAPIClientSuite) TestDialedAddressIsCached(c *tc.C) {
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
	conn, err := juju.NewAPIConnection(context.Background(), juju.NewAPIConnectionParams{
		ControllerStore: store,
		ControllerName:  "foo",
		DialOpts: api.DialOpts{
			DialWebsocket: func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
				apiConn := testRootAPI{
					serverAddrs: params.FromProviderHostsPorts([]network.ProviderHostPorts{{
						network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("example3").AsProviderAddress(), NetPort: 3333},
						network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("example4").AsProviderAddress(), NetPort: 4444},
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

func (s *NewAPIClientSuite) TestWithExistingDNSCache(c *tc.C) {
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
	conn, err := juju.NewAPIConnection(context.Background(), juju.NewAPIConnectionParams{
		ControllerStore: store,
		ControllerName:  "foo",
		DialOpts: api.DialOpts{
			DialWebsocket: func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
				apiConn := testRootAPI{
					serverAddrs: params.FromProviderHostsPorts([]network.ProviderHostPorts{{
						network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("example3").AsProviderAddress(), NetPort: 3333},
						network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("example5").AsProviderAddress(), NetPort: 5555},
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

func (s *NewAPIClientSuite) TestEndpointFiltering(c *tc.C) {
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
		network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("0.1.2.3").AsProviderAddress(), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("2001:db8::1").AsProviderAddress(), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("10.0.0.1").AsProviderAddress(), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
		network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("169.254.1.1").AsProviderAddress(), NetPort: 1234},
		//Duplicate
		network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("0.1.2.3").AsProviderAddress(), NetPort: 1234},
		//Duplicate host, same IP.
		network.ProviderHostPort{ProviderAddress: network.NewMachineAddress("0.1.2.3").AsProviderAddress(), NetPort: 1235},
	}})

	conn, err := juju.NewAPIConnection(context.Background(), juju.NewAPIConnectionParams{
		ControllerStore: store,
		ControllerName:  "foo",
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

// setupControllerWithPathSegment sets up a controller that is
// hosted on a path and returns a connection to it. A modelUUID
// can be specified to return a connection to the model api endpoint.
func setupControllerWithPathSegment(c *tc.C, store *jujuclient.MemStore, modelUUID string) api.Connection {
	err := store.AddController("foo", jujuclient.ControllerDetails{
		ControllerUUID: fakeUUID,
		APIEndpoints: []string{
			"example1:1111/foo",
		},
		DNSCache: map[string][]string{
			"example1": {"0.1.1.1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	conn, err := juju.NewAPIConnection(context.Background(), juju.NewAPIConnectionParams{
		ControllerStore: store,
		ControllerName:  "foo",
		DialOpts: api.DialOpts{
			DialWebsocket: func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
				apiConn := testRootAPI{modelUUID: modelUUID}
				return jsoncodec.NetJSONConn(apitesting.FakeAPIServer(apiConn)), nil
			},
		},
		AccountDetails: new(jujuclient.AccountDetails),
		ModelUUID:      modelUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

func (s *NewAPIClientSuite) TestAPIWithControllerPathSegment(c *tc.C) {
	store := jujuclient.NewMemStore()
	controllerConn := setupControllerWithPathSegment(c, store, "")
	defer controllerConn.Close()
	details, err := store.ControllerByName("foo")
	c.Assert(err, jc.ErrorIsNil)
	// The API address should still contain the path segment.
	c.Assert(details.APIEndpoints, jc.DeepEquals, []string{
		"example1:1111/foo",
	})
}

func (s *NewAPIClientSuite) TestStreamWithControllerPathSegment(c *tc.C) {
	testCases := []struct {
		desc        string
		modelUUID   string
		expectedURL string
	}{
		{
			desc:        "Stream to controller endpoint",
			expectedURL: "wss://example1:1111/foo/bar",
		},
		{
			desc:        "Stream to model endpoint",
			modelUUID:   "9c8fc580-7ad2-43a0-a0b9-c14b80172190",
			expectedURL: "wss://example1:1111/foo/model/9c8fc580-7ad2-43a0-a0b9-c14b80172190/bar",
		},
	}
	for _, tC := range testCases {
		c.Logf("test case: %s", tC.desc)
		store := jujuclient.NewMemStore()
		controllerConn := setupControllerWithPathSegment(c, store, tC.modelUUID)
		defer controllerConn.Close()

		catcher := api.UrlCatcher{}
		s.PatchValue(&api.WebsocketDial, catcher.RecordLocation)

		stream, err := controllerConn.ConnectStream(context.Background(), "/bar", nil)
		c.Assert(err, jc.ErrorIsNil)
		defer stream.Close()

		c.Assert(catcher.Location(), tc.Equals, tC.expectedURL)
	}
}

func (s *NewAPIClientSuite) TestHTTPClientWithControllerPathSegment(c *tc.C) {
	store := jujuclient.NewMemStore()
	conn := setupControllerWithPathSegment(c, store, "9c8fc580-7ad2-43a0-a0b9-c14b80172190")
	defer conn.Close()

	client, err := conn.HTTPClient()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(client.BaseURL, tc.Equals, "https://example1:1111/foo/model/9c8fc580-7ad2-43a0-a0b9-c14b80172190")

	client, err = conn.RootHTTPClient()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(client.BaseURL, tc.Equals, "https://example1:1111/foo")
}

type testRootAPI struct {
	serverAddrs [][]params.HostPort
	modelUUID   string
}

func (r testRootAPI) Admin(id string) (testAdminAPI, error) {
	return testAdminAPI{r: r}, nil
}

type testAdminAPI struct {
	r testRootAPI
}

func (a testAdminAPI) Login(req params.LoginRequest) params.LoginResult {
	loginResult := params.LoginResult{
		ControllerTag: names.NewControllerTag(fakeUUID).String(),
		Servers:       a.r.serverAddrs,
		ServerVersion: version.Current.String(),
	}
	if a.r.modelUUID != "" {
		loginResult.ModelTag = names.NewModelTag(a.r.modelUUID).String()
	}
	return loginResult
}

func checkCommonAPIInfoAttrs(c *tc.C, apiInfo *api.Info, opts api.DialOpts) {
	opts.DNSCache = nil
	c.Check(apiInfo.Tag, tc.Equals, names.NewUserTag("admin"))
	c.Check(apiInfo.CACert, tc.Equals, "certificate")
	c.Check(apiInfo.Password, tc.Equals, "hunter2")
	c.Check(opts, tc.DeepEquals, api.DefaultDialOpts())
}

// newClientStore returns a client store that contains information
// based on the given controller name and info.
func newClientStore(c *tc.C, controllerName string) *jujuclient.MemStore {
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
	c *tc.C,
	controller, model string,
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
) (api.Connection, error) {
	args := juju.NewAPIConnectionParams{
		ControllerStore: store,
		ControllerName:  controller,
		DialOpts:        api.DefaultDialOpts(),
		OpenAPI:         apiOpen,
		AccountDetails:  &jujuclient.AccountDetails{},
	}
	accountDetails, err := store.AccountDetails(controller)
	if !errors.Is(err, errors.NotFound) {
		c.Assert(err, jc.ErrorIsNil)
		args.AccountDetails = accountDetails
	}
	if model != "" {
		modelDetails, err := store.ModelByName(controller, model)
		c.Assert(err, jc.ErrorIsNil)
		args.ModelUUID = modelDetails.ModelUUID
	}
	return juju.NewAPIConnection(context.Background(), args)
}

type ipAddrResolverFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

func (f ipAddrResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func (s *NewAPIClientSuite) TestUpdateControllerDetailsFromLogin(c *tc.C) {
	// These tests currently focus solely focus on asserting the
	// controller's API endpoints are correctly updated.
	tests := []struct {
		description                 string
		controllerName              string
		updateDetails               juju.UpdateControllerParams
		expectedControllerEndpoints []string
	}{{
		description:    "Empty connected address",
		controllerName: "test-controller",
		updateDetails: juju.UpdateControllerParams{
			CurrentHostPorts: []network.MachineHostPorts{
				network.NewMachineHostPorts(1234, "31.0.0.1"),
			},
		},
		expectedControllerEndpoints: []string{"31.0.0.1:1234"},
	}, {
		description:    "Populated connected address",
		controllerName: "test-controller",
		updateDetails: juju.UpdateControllerParams{
			CurrentHostPorts: []network.MachineHostPorts{
				network.NewMachineHostPorts(1234, "31.0.0.1"),
			},
			CurrentConnection: &juju.CurrentConnection{
				Proxied: false,
				Address: &url.URL{
					Host: "mycontroller:5432",
				},
				IPAddress: "31.1.1.1:1234",
			},
		},
		expectedControllerEndpoints: []string{"mycontroller:5432", "31.0.0.1:1234"},
	}, {
		description:    "Populated connected address that is proxied",
		controllerName: "test-controller",
		updateDetails: juju.UpdateControllerParams{
			CurrentHostPorts: []network.MachineHostPorts{
				network.NewMachineHostPorts(1234, "31.0.0.1"),
			},
			CurrentConnection: &juju.CurrentConnection{
				Proxied: true,
				Address: &url.URL{
					Host: "mycontroller:5432",
				},
				IPAddress: "10.1.2.3",
			},
		},
		expectedControllerEndpoints: []string{"31.0.0.1:1234"},
	}, {
		description:    "Duplicate and local IP addresses are removed",
		controllerName: "test-controller",
		updateDetails: juju.UpdateControllerParams{
			CurrentHostPorts: []network.MachineHostPorts{
				network.NewMachineHostPorts(1234, "31.0.0.1"),
				network.NewMachineHostPorts(1234, "31.0.0.1"),
				network.NewMachineHostPorts(1234, "127.0.0.1"),
			},
		},
		expectedControllerEndpoints: []string{"31.0.0.1:1234"},
	}}

	for i, test := range tests {
		c.Logf("running test case %d - %s", i, test.description)
		store := &testClientStore{
			controllerDetails: map[string]*jujuclient.ControllerDetails{
				test.controllerName: {},
			},
		}

		err := juju.UpdateControllerDetailsFromLogin(store, test.controllerName, test.updateDetails)
		c.Assert(err, tc.IsNil)

		controller := store.controllerDetails[test.controllerName]
		c.Assert(controller.APIEndpoints, tc.DeepEquals, test.expectedControllerEndpoints)
	}

}

type testClientStore struct {
	jujuclient.ClientStore

	mu                sync.RWMutex
	accountDetails    map[string]*jujuclient.AccountDetails
	controllerDetails map[string]*jujuclient.ControllerDetails
}

func (s *testClientStore) AccountDetails(controllerName string) (*jujuclient.AccountDetails, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.accountDetails[controllerName] == nil {
		return nil, errors.NotFound
	}
	return s.accountDetails[controllerName], nil
}

func (s *testClientStore) UpdateAccount(controllerName string, accountDetails jujuclient.AccountDetails) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.accountDetails == nil {
		return errors.NotImplemented
	}
	s.accountDetails[controllerName] = &accountDetails
	return nil
}

func (s *testClientStore) ControllerByName(controllerName string) (*jujuclient.ControllerDetails, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.controllerDetails[controllerName] == nil {
		return nil, errors.NotFound
	}
	return s.controllerDetails[controllerName], nil
}

func (s *testClientStore) UpdateController(controllerName string, details jujuclient.ControllerDetails) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.controllerDetails == nil {
		return errors.NotImplemented
	}
	s.controllerDetails[controllerName] = &details
	return nil
}
