// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/unitassigner"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/network"
	"github.com/juju/utils/set"
)

// Info encapsulates information about a server holding juju state and
// can be used to make a connection to it.
type Info struct {

	// This block of fields is sufficient to connect:

	// Addrs holds the addresses of the controllers.
	Addrs []string

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
}

// Ports returns the unique ports for the api addresses.
func (info *Info) Ports() []int {
	ports := set.NewInts()
	hostPorts, err := network.ParseHostPorts(info.Addrs...)
	if err != nil {
		// Addresses have already been validated.
		panic(err)
	}
	for _, hp := range hostPorts {
		ports.Add(hp.Port)
	}
	return ports.Values()
}

// Validate validates the API info.
func (info *Info) Validate() error {
	if len(info.Addrs) == 0 {
		return errors.NotValidf("missing addresses")
	}
	if _, err := network.ParseHostPorts(info.Addrs...); err != nil {
		return errors.NotValidf("host addresses: %v", err)
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
// dumb copy of all the methods on api.state; we can and should be extracting
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
	Login(name names.Tag, password, nonce string, ms []macaroon.Slice) error
	ServerVersion() (version.Number, bool)

	// APICaller provides the facility to make API calls directly.
	// This should not be used outside the api/* packages or tests.
	base.APICaller

	// ControllerTag returns the tag of the controller.
	// This could be defined on base.APICaller.
	ControllerTag() names.ControllerTag

	// All the rest are strange and questionable and deserve extra attention
	// and/or discussion.

	// Something-or-other expects Ping to exist, and *maybe* the heartbeat
	// *should* be handled outside the State type, but it's also handled
	// inside it as well. We should figure this out sometime -- we should
	// either expose Ping() or Broken() but not both.
	Ping() error

	// I think this is actually dead code. It's tested, at least, so I'm
	// keeping it for now, but it's not apparently used anywhere else.
	AllFacadeVersions() map[string][]int

	// AuthTag returns the tag of the authorized user of the state API
	// connection.
	AuthTag() names.Tag

	// ModelAccess returns the access level of authorized user to the model.
	ModelAccess() string

	// ControllerAccess returns the access level of authorized user to the controller.
	ControllerAccess() string

	// These methods expose a bunch of worker-specific facades, and basically
	// just should not exist; but removing them is too noisy for a single CL.
	// Client in particular is intimately coupled with State -- and the others
	// will be easy to remove, but until we're using them via manifolds it's
	// prohibitively ugly to do so.
	Client() *Client
	Uniter() (*uniter.State, error)
	Upgrader() *upgrader.State
	Reboot() (reboot.State, error)
	DiscoverSpaces() *discoverspaces.API
	InstancePoller() *instancepoller.API
	CharmRevisionUpdater() *charmrevisionupdater.State
	Cleaner() *cleaner.API
	MetadataUpdater() *imagemetadata.Client
	UnitAssigner() unitassigner.API
}
