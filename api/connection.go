// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"strconv"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/core/network"
	jujuproxy "github.com/juju/juju/internal/proxy"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/jsoncodec"
)

// conn is the internal implementation of the Connection interface.
type conn struct {
	client rpcConnection
	conn   jsoncodec.JSONConn
	clock  clock.Clock

	// addr is the URL used to connect to the root of the API server.
	addr *url.URL

	// ipAddr is the IP address used to connect to the API server.
	ipAddr string

	// cookieURL is the URL that HTTP cookies for the API
	// will be associated with (specifically macaroon auth cookies).
	cookieURL *url.URL

	// modelTag holds the model tag.
	// It is empty if there is no model tag associated with the connection.
	modelTag names.ModelTag

	// controllerTag holds the controller's tag once we're connected.
	controllerTag names.ControllerTag

	// serverVersion holds the version of the API server that we are
	// connected to.  It is possible that this version is 0 if the
	// server does not report this during login.
	serverVersion version.Number

	// hostPorts is the API server addresses returned from Login,
	// which the client may cache and use for fail-over.
	hostPorts []network.MachineHostPorts

	// publicDNSName is the public host name returned from Login
	// which the client can use to make a connection verified
	// by an officially signed certificate.
	publicDNSName string

	// facadeVersions holds the versions of all facades as reported by
	// Login
	facadeVersions map[string][]int

	// pingFacadeVersion is the version to use for the pinger. This is lazily
	// set at initialization to avoid a race in our tests. See
	// http://pad.lv/1614732 for more details regarding the race.
	pingerFacadeVersion int

	// authTag holds the authenticated entity's tag after login.
	authTag names.Tag

	// mpdelAccess holds the access level of the user to the connected model.
	modelAccess string

	// controllerAccess holds the access level of the user to the connected controller.
	controllerAccess string

	// broken is a channel that gets closed when the connection is
	// broken.
	broken chan struct{}

	// closed is a channel that gets closed when State.Close is called.
	closed chan struct{}

	// loggedIn holds whether the client has successfully logged
	// in. It's a int32 so that the atomic package can be used to
	// access it safely.
	loggedIn int32

	// loginProvider holds the provider used for login.
	loginProvider LoginProvider

	// serverScheme is the URI scheme of the API Server
	serverScheme string

	// tlsConfig holds the TLS config appropriate for making SSL
	// connections to the API endpoints.
	tlsConfig *tls.Config

	// bakeryClient holds the client that will be used to
	// authorize macaroon based login requests.
	bakeryClient *httpbakery.Client

	// proxier is the proxier used for this connection when not nil. If's expected
	// the proxy has already been started when placing in this var. This struct
	// will take the responsibility of closing the proxy.
	proxier jujuproxy.Proxier
}

// Login implements the Login method of the Connection interface providing authentication
// using basic auth or macaroons.
//
// TODO (alesstimec, wallyworld): This method should be removed and
// a login provider should be used instead.
func (c *conn) Login(ctx context.Context, name names.Tag, password, nonce string, ms []macaroon.Slice) error {
	lp := NewLegacyLoginProvider(name, password, nonce, ms, c.bakeryClient, c.cookieURL)
	result, err := lp.Login(ctx, c)
	if err != nil {
		return errors.Trace(err)
	}
	return c.setLoginResult(result)
}

func (c *conn) setLoginResult(p *LoginResultParams) error {
	c.authTag = p.tag
	c.serverVersion = p.serverVersion
	var modelTag names.ModelTag
	if p.modelTag != "" {
		var err error
		modelTag, err = names.ParseModelTag(p.modelTag)
		if err != nil {
			return errors.Annotatef(err, "invalid model tag in login result")
		}
	}
	if modelTag.Id() != c.modelTag.Id() {
		return errors.Errorf("mismatched model tag in login result (got %q want %q)", modelTag.Id(), c.modelTag.Id())
	}
	ctag, err := names.ParseControllerTag(p.controllerTag)
	if err != nil {
		return errors.Annotatef(err, "invalid controller tag %q returned from login", p.controllerTag)
	}
	c.controllerTag = ctag
	c.controllerAccess = p.controllerAccess
	c.modelAccess = p.modelAccess

	hostPorts := p.servers
	// If the connection is not proxied then we will add the connection address
	// to host ports. Additionally if the connection address includes a path,
	// it can't be added to host ports as it will lose the path so skip the
	// address in scenarios where we connect through a load-balancer.
	if !c.IsProxied() && c.addr.Path == "" {
		hostPorts, err = addAddress(p.servers, c.addr.Host)
		if err != nil {
			if clerr := c.Close(); clerr != nil {
				err = errors.Annotatef(err, "error closing conn: %v", clerr)
			}
			return err
		}
	}
	c.hostPorts = hostPorts
	c.publicDNSName = p.publicDNSName

	c.facadeVersions = make(map[string][]int, len(p.facades))
	for _, facade := range p.facades {
		c.facadeVersions[facade.Name] = facade.Versions
	}

	c.setLoggedIn()
	return nil
}

// AuthTag returns the tag of the authorized user of the conn API connection.
func (c *conn) AuthTag() names.Tag {
	return c.authTag
}

// ControllerAccess returns the access level of authorized user to the model.
func (c *conn) ControllerAccess() string {
	return c.controllerAccess
}

// CookieURL returns the URL that HTTP cookies for the API will be
// associated with.
func (c *conn) CookieURL() *url.URL {
	copy := *c.cookieURL
	return &copy
}

// slideAddressToFront moves the address at the location (serverIndex, addrIndex) to be
// the first address of the first server.
func slideAddressToFront(servers []network.MachineHostPorts, serverIndex, addrIndex int) {
	server := servers[serverIndex]
	hostPort := server[addrIndex]
	// Move the matching address to be the first in this server
	for ; addrIndex > 0; addrIndex-- {
		server[addrIndex] = server[addrIndex-1]
	}
	server[0] = hostPort
	for ; serverIndex > 0; serverIndex-- {
		servers[serverIndex] = servers[serverIndex-1]
	}
	servers[0] = server
}

// addAddress appends a new server derived from the given
// address to servers if the address is not already found
// there.
func addAddress(servers []network.MachineHostPorts, addr string) ([]network.MachineHostPorts, error) {
	for i, server := range servers {
		for j, hostPort := range server {
			if network.DialAddress(hostPort) == addr {
				slideAddressToFront(servers, i, j)
				return servers, nil
			}
		}
	}
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return nil, err
	}
	result := make([]network.MachineHostPorts, 0, len(servers)+1)
	result = append(result, network.NewMachineHostPorts(port, host))
	result = append(result, servers...)
	return result, nil
}

// KeyUpdater returns access to the KeyUpdater API
func (c *conn) KeyUpdater() *keyupdater.Client {
	return keyupdater.NewClient(c)
}

// ServerVersion holds the version of the API server that we are connected to.
// It is possible that this version is Zero if the server does not report this
// during login. The second result argument indicates if the version number is
// set.
func (c *conn) ServerVersion() (version.Number, bool) {
	return c.serverVersion, c.serverVersion != version.Zero
}
