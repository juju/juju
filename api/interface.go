// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"time"

	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/api/deployer"
	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/unitassigner"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/version"
)

// Info encapsulates information about a server holding juju state and
// can be used to make a connection to it.
type Info struct {

	// This block of fields is sufficient to connect:

	// Addrs holds the addresses of the controllers.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string

	// ModelTag holds the model tag for the model we are
	// trying to connect to.
	ModelTag names.ModelTag

	// ...but this block of fields is all about the authentication mechanism
	// to use after connecting -- if any -- and should probably be extracted.

	// UseMacaroons, when true, enables macaroon-based login and ignores
	// the provided username and password.
	UseMacaroons bool `yaml:"use-macaroons,omitempty"`

	// Tag holds the name of the entity that is connecting.
	// If this is nil, and the password is empty, no login attempt will be made.
	// (this is to allow tests to access the API to check that operations
	// fail when not logged in).
	Tag names.Tag

	// Password holds the password for the administrator or connecting entity.
	Password string

	// Nonce holds the nonce used when provisioning the machine. Used
	// only by the machine agent.
	Nonce string `yaml:",omitempty"`
}

// DialOpts holds configuration parameters that control the
// Dialing behavior when connecting to a controller.
type DialOpts struct {
	// DialAddressInterval is the amount of time to wait
	// before starting to dial another address.
	DialAddressInterval time.Duration

	// Timeout is the amount of time to wait contacting
	// a controller.
	Timeout time.Duration

	// RetryDelay is the amount of time to wait between
	// unsucssful connection attempts.
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

// OpenFunc is the usual form of a function that opens an API connection.
type OpenFunc func(*Info, DialOpts) (Connection, error)

// Connection exists purely to make api-opening funcs mockable. It's just a
// dumb copy of all the methods on api.Connection; we can and should be extracting
// smaller and more relevant interfaces (and dropping some of them too).

// Connection represents a connection to a Juju API server.
type Connection interface {

	// This first block of methods is pretty close to a sane Connection interface.
	Close() error
	Broken() <-chan struct{}
	Addr() string
	APIHostPorts() [][]network.HostPort

	// These are a bit off -- ServerVersion is apparently not known until after
	// Login()? Maybe evidence of need for a separate AuthenticatedConnection..?
	Login(name names.Tag, password, nonce string) error
	ServerVersion() (version.Number, bool)

	// APICaller provides the facility to make API calls directly.
	// This should not be used outside the api/* packages or tests.
	base.APICaller

	// ControllerTag returns the model tag of the controller
	// (as opposed to the model tag of the currently connected
	// model inside that controller).
	// This could be defined on base.APICaller.
	ControllerTag() (names.ModelTag, error)

	// All the rest are strange and questionable and deserve extra attention
	// and/or discussion.

	// Something-or-other expects Ping to exist, and *maybe* the heartbeat
	// *should* be handled outside the State type, but it's also handled
	// inside it as well. We should figure this out sometime -- we should
	// either expose Ping() or Broken() but not both.
	Ping() error

	// RPCClient is apparently exported for testing purposes only, but this
	// seems to indicate *some* sort of layering confusion.
	RPCClient() *rpc.Conn

	// I think this is actually dead code. It's tested, at least, so I'm
	// keeping it for now, but it's not apparently used anywhere else.
	AllFacadeVersions() map[string][]int

	// These methods expose a bunch of worker-specific facades, and basically
	// just should not exist; but removing them is too noisy for a single CL.
	// Client in particular is intimately coupled with State -- and the others
	// will be easy to remove, but until we're using them via manifolds it's
	// prohibitively ugly to do so.
	Client() *Client
	Machiner() *machiner.State
	Provisioner() *provisioner.State
	Uniter() (*uniter.State, error)
	Firewaller() *firewaller.State
	Agent() *agent.State
	Upgrader() *upgrader.State
	Reboot() (reboot.State, error)
	Deployer() *deployer.State
	Addresser() *addresser.API
	DiscoverSpaces() *discoverspaces.API
	InstancePoller() *instancepoller.API
	CharmRevisionUpdater() *charmrevisionupdater.State
	Cleaner() *cleaner.API
	MetadataUpdater() *imagemetadata.Client
	UnitAssigner() unitassigner.API
}
