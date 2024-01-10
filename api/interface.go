// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/url"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/proxy"
	"github.com/juju/juju/rpc/jsoncodec"
)

// AnonymousUsername is the special username to use for anonymous logins.
const AnonymousUsername = "jujuanonymous"

// Info encapsulates information about a server holding juju conn and
// can be used to make a connection to it.
type Info struct {

	// This block of fields is sufficient to connect:

	// Addrs holds the addresses of the controllers.
	Addrs []string

	// ControllerUUID is the UUID of the controller.
	ControllerUUID string

	// SNIHostName optionally holds the host name to use for
	// server name indication (SNI) when connecting
	// to the addresses in Addrs above. If CACert is non-empty,
	// this field is ignored.
	SNIHostName string

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	// If this is empty, the standard system root certificates
	// will be used.
	CACert string

	// ModelTag holds the model tag for the model we are
	// trying to connect to. If this is empty, a controller-only
	// login will be made.
	ModelTag names.ModelTag

	// ...but this block of fields is all about the authentication mechanism
	// to use after connecting -- if any -- and should probably be extracted.

	// SkipLogin, if true, skips the Login call on connection. It is an
	// error to set Tag, Password, or Macaroons if SkipLogin is true.
	SkipLogin bool `yaml:"-"`

	// Tag holds the name of the entity that is connecting.
	// If this is nil, and the password is empty, macaroon authentication
	// will be used to log in unless SkipLogin is true.
	Tag names.Tag

	// Password holds the password for the administrator or connecting entity.
	Password string

	// Macaroons holds a slice of macaroon.Slice that may be used to
	// authenticate with the API server.
	Macaroons []macaroon.Slice `yaml:",omitempty"`

	// Nonce holds the nonce used when provisioning the machine. Used
	// only by the machine agent.
	Nonce string `yaml:",omitempty"`

	// Proxier describes a proxier to use to for establing an API connection
	// A nil proxier means that it will not be used.
	Proxier proxy.Proxier
}

// Ports returns the unique ports for the api addresses.
func (info *Info) Ports() []int {
	ports := set.NewInts()
	for _, addr := range info.Addrs {
		hp, err := network.ParseMachineHostPort(addr)
		if err != nil {
			// Addresses have already been validated.
			panic(err)
		}
		ports.Add(hp.Port())
	}
	return ports.Values()
}

// Validate validates the API info.
func (info *Info) Validate() error {
	if len(info.Addrs) == 0 {
		return errors.NotValidf("missing addresses")
	}

	for _, addr := range info.Addrs {
		_, err := network.ParseMachineHostPort(addr)
		if err != nil {
			return errors.NotValidf("host addresses: %v", err)
		}
	}

	if info.SkipLogin {
		if info.Tag != nil {
			return errors.NotValidf("specifying Tag and SkipLogin")
		}
		if info.Password != "" {
			return errors.NotValidf("specifying Password and SkipLogin")
		}
		if len(info.Macaroons) > 0 {
			return errors.NotValidf("specifying Macaroons and SkipLogin")
		}
	}
	return nil
}

// DialOpts holds configuration parameters that control the
// Dialing behavior when connecting to a controller.
type DialOpts struct {
	// DialAddressInterval is the amount of time to wait
	// before starting to dial another address.
	DialAddressInterval time.Duration

	// DialTimeout is the amount of time to wait for the dial
	// portion only of the api.Open to succeed. If this is zero,
	// there is no dial timeout.
	DialTimeout time.Duration

	// Timeout is the amount of time to wait for the entire
	// api.Open to succeed (including dial and login). If this is
	// zero, there is no timeout.
	Timeout time.Duration

	// RetryDelay is the amount of time to wait between
	// unsuccessful connection attempts. If this is
	// zero, only one attempt will be made.
	RetryDelay time.Duration

	// BakeryClient is the httpbakery Client, which
	// is used to do the macaroon-based authorization.
	// This and the *http.Client inside it are copied
	// by Open, and any RoundTripper field
	// the HTTP client is ignored.
	BakeryClient *httpbakery.Client

	// InsecureSkipVerify skips TLS certificate verification
	// when connecting to the controller. This should only
	// be used in tests, or when verification cannot be
	// performed and the communication need not be secure.
	InsecureSkipVerify bool

	// DialWebsocket is used to make connections to API servers.
	// It will be called with a websocket URL to connect to,
	// and the TLS configuration to use to secure the connection.
	// If ipAddr is non-empty, the actual net.Dial should use
	// that IP address, regardless of the URL host.
	//
	// If DialWebsocket is nil, a default implementation using
	// gorilla websockets will be used.
	DialWebsocket func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error)

	// IPAddrResolver is used to resolve host names to IP addresses.
	// If it is nil, net.DefaultResolver will be used.
	IPAddrResolver IPAddrResolver

	// DNSCache is consulted to find and store cached DNS lookups.
	// If it is nil, no cache will be used or updated.
	DNSCache DNSCache

	// Clock is used as a time source for retries.
	// If it is nil, clock.WallClock will be used.
	Clock clock.Clock

	// VerifyCA is an optional callback that is invoked by the dialer when
	// the remote server presents a CA certificate that cannot be
	// automatically verified. If the callback returns a non-nil error then
	// the connection attempt will be aborted.
	VerifyCA func(host, endpoint string, caCert *x509.Certificate) error
}

// IPAddrResolver implements a resolved from host name to the
// set of IP addresses associated with it. It is notably
// implemented by net.Resolver.
type IPAddrResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DNSCache implements a cache of DNS lookup results.
type DNSCache interface {
	// Lookup returns the IP addresses associated
	// with the given host.
	Lookup(host string) []string
	// Add sets the IP addresses associated with
	// the given host name.
	Add(host string, ips []string)
}

// DefaultDialOpts returns a DialOpts representing the default
// parameters for contacting a controller.
func DefaultDialOpts() DialOpts {
	return DialOpts{
		DialAddressInterval: 50 * time.Millisecond,
		Timeout:             10 * time.Minute,
		RetryDelay:          2 * time.Second,
	}
}

// DialOption is the type of functions that mutate DialOpts
type DialOption func(*DialOpts)

// OpenFunc is the usual form of a function that opens an API connection.
type OpenFunc func(*Info, DialOpts) (Connection, error)

// Connection exists purely to make api-opening funcs mockable. It's just a
// dumb copy of all the methods on api.conn; we can and should be extracting
// smaller and more relevant interfaces (and dropping some of them too).

// Connection represents a connection to a Juju API server.
type Connection interface {

	// This first block of methods is pretty close to a sane Connection interface.

	// Close closes the connection.
	Close() error

	// Addr returns the address used to connect to the API server.
	Addr() string

	// IPAddr returns the IP address used to connect to the API server.
	IPAddr() string

	// APIHostPorts returns addresses that may be used to connect
	// to the API server, including the address used to connect.
	//
	// The addresses are scoped (public, cloud-internal, etc.), so
	// the client may choose which addresses to attempt. For the
	// Juju CLI, all addresses must be attempted, as the CLI may
	// be invoked both within and outside the model (think
	// private clouds).
	APIHostPorts() []network.MachineHostPorts

	// Broken returns a channel which will be closed if the connection
	// is detected to be broken, either because the underlying
	// connection has closed or because API pings have failed.
	Broken() <-chan struct{}

	// IsBroken returns whether the connection is broken. It checks
	// the Broken channel and if that is open, attempts a connection
	// ping.
	IsBroken() bool

	// IsProxied returns weather the connection is proxied.
	IsProxied() bool

	// Proxy returns the Proxier used to establish the connection if one was
	// used at all. If no Proxier was used then it's expected that returned
	// Proxier will be nil. Use IsProxied() to test for the presence of a proxy.
	Proxy() proxy.Proxier

	// PublicDNSName returns the host name for which an officially
	// signed certificate will be used for TLS connection to the server.
	// If empty, the private Juju CA certificate must be used to verify
	// the connection.
	PublicDNSName() string

	// These are a bit off -- ServerVersion is apparently not known until after
	// Login()? Maybe evidence of need for a separate AuthenticatedConnection..?
	Login(ctx context.Context, name names.Tag, password, nonce string, ms []macaroon.Slice) error
	ServerVersion() (version.Number, bool)

	// APICaller provides the facility to make API calls directly.
	// This should not be used outside the api/* packages or tests.
	base.APICaller

	// ControllerTag returns the tag of the controller.
	// This could be defined on base.APICaller.
	ControllerTag() names.ControllerTag

	// All the rest are strange and questionable and deserve extra attention
	// and/or discussion.

	// AuthTag returns the tag of the authorized user of the conn API
	// connection.
	AuthTag() names.Tag

	// ControllerAccess returns the access level of authorized user to the controller.
	ControllerAccess() string

	// CookieURL returns the URL that HTTP cookies for the API will be
	// associated with.
	CookieURL() *url.URL
}
