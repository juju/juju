// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"net/url"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/jsoncodec"
)

var (
	CertDir                 = &certDir
	WebsocketDial           = &websocketDial
	WebsocketDialWithErrors = websocketDialWithErrors
	SlideAddressToFront     = slideAddressToFront
	BestVersion             = bestVersion
	FacadeVersions          = &facadeVersions
	HasHooks                = &hasHooks
)

func DialAPI(info *Info, opts DialOpts) (jsoncodec.JSONConn, string, error) {
	result, err := dialAPI(context.TODO(), info, opts)
	if err != nil {
		return nil, "", err
	}
	// Replace the IP address in the URL with the
	// host name so that tests can check it more
	// easily.
	u, _ := url.Parse(result.urlStr)
	u.Host = result.addr
	return result.conn, u.String(), nil
}

// RPCConnection defines the methods that are called on the rpc.Conn instance.
type RPCConnection rpcConnection

// SetServerAddress allows changing the URL to the internal API server
// that AddLocalCharm uses in order to test NotImplementedError.
func SetServerAddress(c *Client, scheme, addr string) {
	c.st.serverScheme = scheme
	c.st.addr = addr
}

// ServerRoot is exported so that we can test the built URL.
func ServerRoot(c *Client) string {
	return c.st.serverRoot()
}

// UnderlyingConn returns the underlying transport connection.
func UnderlyingConn(c Connection) jsoncodec.JSONConn {
	return c.(*state).conn
}

// TestingStateParams is the parameters for NewTestingState, so that you can
// only set the bits that you actually want to test.
type TestingStateParams struct {
	Address        string
	ModelTag       string
	APIHostPorts   []network.MachineHostPorts
	FacadeVersions map[string][]int
	ServerScheme   string
	ServerRoot     string
	RPCConnection  RPCConnection
	Clock          clock.Clock
	Broken, Closed chan struct{}
}

// NewTestingState creates an api.State object that can be used for testing. It
// isn't backed onto an actual API server, so actual RPC methods can't be
// called on it. But it can be used for testing general behaviour.
func NewTestingState(params TestingStateParams) Connection {
	var modelTag names.ModelTag
	if params.ModelTag != "" {
		t, err := names.ParseModelTag(params.ModelTag)
		if err != nil {
			panic("invalid model tag")
		}
		modelTag = t
	}
	st := &state{
		client:            params.RPCConnection,
		clock:             params.Clock,
		addr:              params.Address,
		modelTag:          modelTag,
		hostPorts:         params.APIHostPorts,
		facadeVersions:    params.FacadeVersions,
		serverScheme:      params.ServerScheme,
		serverRootAddress: params.ServerRoot,
		broken:            params.Broken,
		closed:            params.Closed,
	}
	return st
}

// APIClient returns a 'barebones' api.Client suitable for calling FindTools in
// an error state (anything else is likely to panic.)
func APIClient(apiCaller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(apiCaller, "Client")
	return &Client{ClientFacade: frontend, facade: backend, st: &state{}}
}

// PatchClientFacadeCall changes the internal FacadeCaller to one that lets
// you mock out the FacadeCall method. The function returned by
// PatchClientFacadeCall is a cleanup function that returns the client to its
// original state.
func PatchClientFacadeCall(c *Client, mockCall func(request string, params interface{}, response interface{}) error) func() {
	orig := c.facade
	c.facade = &resultCaller{mockCall}
	return func() {
		c.facade = orig
	}
}

type resultCaller struct {
	mockCall func(request string, params interface{}, response interface{}) error
}

func (f *resultCaller) FacadeCall(request string, params, response interface{}) error {
	return f.mockCall(request, params, response)
}

func (f *resultCaller) Name() string {
	return ""
}

func (f *resultCaller) BestAPIVersion() int {
	return 0
}

type rawAPICaller struct {
	base.APICaller
}

func (r *rawAPICaller) Context() context.Context {
	return context.Background()
}

func (f *resultCaller) RawAPICaller() base.APICaller {
	return &rawAPICaller{}
}

func ExtractMacaroons(conn Connection) ([]macaroon.Slice, error) {
	st, ok := conn.(*state)
	if !ok {
		return nil, errors.Errorf("conn must be a real connection")
	}
	return httpbakery.MacaroonsForURL(st.bakeryClient.Client.Jar, st.cookieURL), nil
}
