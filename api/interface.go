// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"io"
	"net/http"
	"time"

	"github.com/juju/names"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/api/deployer"
	"github.com/juju/juju/api/diskmanager"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/api/keyupdater"
	apilogger "github.com/juju/juju/api/logger"
	"github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/networker"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/resumer"
	"github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/api/storageprovisioner"
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

	// Addrs holds the addresses of the state servers.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string

	// EnvironTag holds the environ tag for the environment we are
	// trying to connect to.
	EnvironTag names.EnvironTag

	// ...but this block of fields is all about the authentication mechanism
	// to use after connecting -- if any -- and should probably be extracted.

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
// Dialing behavior when connecting to a state server.
type DialOpts struct {
	// DialAddressInterval is the amount of time to wait
	// before starting to dial another address.
	DialAddressInterval time.Duration

	// Timeout is the amount of time to wait contacting
	// a state server.
	Timeout time.Duration

	// RetryDelay is the amount of time to wait between
	// unsucssful connection attempts.
	RetryDelay time.Duration
}

// DefaultDialOpts returns a DialOpts representing the default
// parameters for contacting a state server.
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
type Connection interface {

	// This first block of methods is pretty close to a sane Connection interface.
	Close() error
	Broken() <-chan struct{}
	Addr() string
	APIHostPorts() [][]network.HostPort

	// These are a bit off -- ServerVersion is apparently not known until after
	// Login()? Maybe evidence of need for a separate AuthenticatedConnection..?
	Login(name, password, nonce string) error
	ServerVersion() (version.Number, bool)

	// These are either part of base.APICaller or look like they probably should
	// be (ServerTag in particular). It's fine and good for Connection to be an
	// APICaller.
	APICall(facade string, version int, id, method string, args, response interface{}) error
	BestFacadeVersion(string) int
	EnvironTag() (names.EnvironTag, error)
	ServerTag() (names.EnvironTag, error)

	// These HTTP methods should probably be separated out somehow.
	NewHTTPClient() *http.Client
	NewHTTPRequest(method, path string) (*http.Request, error)
	SendHTTPRequest(path string, args interface{}) (*http.Request, *http.Response, error)
	SendHTTPRequestReader(path string, attached io.Reader, meta interface{}, name string) (*http.Request, *http.Response, error)

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
	Resumer() *resumer.API
	Networker() networker.State
	Provisioner() *provisioner.State
	Uniter() (*uniter.State, error)
	DiskManager() (*diskmanager.State, error)
	StorageProvisioner(scope names.Tag) *storageprovisioner.State
	Firewaller() *firewaller.State
	Agent() *agent.State
	Upgrader() *upgrader.State
	Reboot() (*reboot.State, error)
	Deployer() *deployer.State
	Environment() *environment.Facade
	Logger() *apilogger.State
	KeyUpdater() *keyupdater.State
	Addresser() *addresser.API
	InstancePoller() *instancepoller.API
	CharmRevisionUpdater() *charmrevisionupdater.State
	Cleaner() *cleaner.API
	Rsyslog() *rsyslog.State
}
