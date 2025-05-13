// Copyright 2012, 2013, 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	jujuproxy "github.com/juju/juju/internal/proxy"
	"github.com/juju/juju/rpc/jsoncodec"
)

var (
	CertDir             = &certDir
	SlideAddressToFront = slideAddressToFront
	FacadeVersions      = &facadeVersions

	LoginDeviceAPICall                = &loginDeviceAPICall
	GetDeviceSessionTokenAPICall      = &getDeviceSessionTokenAPICall
	LoginWithSessionTokenAPICall      = &loginWithSessionTokenAPICall
	LoginWithClientCredentialsAPICall = &loginWithClientCredentialsAPICall
)

func DialAPI(info *Info, opts DialOpts) (jsoncodec.JSONConn, string, error) {
	result, err := dialAPI(context.Background(), info, opts)
	if err != nil {
		return nil, "", err
	}
	return result.conn, result.dialAddr.String(), nil
}

// CookieURL returns the cookie URL of the connection.
func CookieURL(c Connection) *url.URL {
	return c.(*conn).cookieURL
}

// UnderlyingConn returns the underlying transport connection.
func UnderlyingConn(c Connection) jsoncodec.JSONConn {
	return c.(*conn).conn
}

// RPCConnection defines the methods that are called on the rpc.Conn instance.
type RPCConnection rpcConnection

// TestingConnectionParams is the parameters for NewTestingConnection, so that you can
// only set the bits that you actually want to test.
type TestingConnectionParams struct {
	Address        string
	ModelTag       string
	APIHostPorts   []network.MachineHostPorts
	FacadeVersions map[string][]int
	ServerScheme   string
	ServerRoot     string
	RPCConnection  RPCConnection
	Clock          clock.Clock
	Broken, Closed chan struct{}
	Proxier        jujuproxy.Proxier
}

// NewTestingConnection creates an api.Connection object that can be used for testing.
func NewTestingConnection(c *tc.C, params TestingConnectionParams) Connection {
	var modelTag names.ModelTag
	if params.ModelTag != "" {
		t, err := names.ParseModelTag(params.ModelTag)
		c.Assert(err, tc.IsNil)
		modelTag = t
	}
	url := &url.URL{}
	if params.Address != "" {
		var err error
		url, err = url.Parse(params.Address)
		c.Assert(err, tc.IsNil)
		c.Assert(url.Scheme, tc.Not(tc.Equals), "")
		c.Assert(url.Host, tc.Not(tc.Equals), "")
		c.Assert(url.Port(), tc.Not(tc.Equals), "")
	}
	conn := &conn{
		client:         params.RPCConnection,
		clock:          params.Clock,
		addr:           url,
		modelTag:       modelTag,
		hostPorts:      params.APIHostPorts,
		facadeVersions: params.FacadeVersions,
		serverScheme:   params.ServerScheme,
		broken:         params.Broken,
		closed:         params.Closed,
		proxier:        params.Proxier,
		bakeryClient:   httpbakery.NewClient(),
	}
	return conn
}
