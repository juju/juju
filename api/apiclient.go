// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"strings"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/parallel"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.api")

// PingPeriod defines how often the internal connection health check
// will run. It's a variable so it can be changed in tests.
var PingPeriod = 1 * time.Minute

type State struct {
	client *rpc.Conn
	conn   *websocket.Conn

	// addr is the address used to connect to the API server.
	addr string

	// environTag holds the environment tag once we're connected
	environTag string

	// serverTag holds the server tag once we're connected.
	// This is only set with newer apiservers where they are using
	// the v1 login mechansim.
	serverTag string

	// serverVersion holds the version of the API server that we are
	// connected to.  It is possible that this version is 0 if the
	// server does not report this during login.
	serverVersion version.Number

	// hostPorts is the API server addresses returned from Login,
	// which the client may cache and use for failover.
	hostPorts [][]network.HostPort

	// facadeVersions holds the versions of all facades as reported by
	// Login
	facadeVersions map[string][]int

	// authTag holds the authenticated entity's tag after login.
	authTag names.Tag

	// broken is a channel that gets closed when the connection is
	// broken.
	broken chan struct{}

	// closed is a channel that gets closed when State.Close is called.
	closed chan struct{}

	// tag and password hold the cached login credentials.
	tag      string
	password string

	// serverRoot holds the cached API server address and port we used
	// to login, with a https:// prefix.
	serverRoot string

	// certPool holds the cert pool that is used to authenticate the tls
	// connections to the API.
	certPool *x509.CertPool
}

// Info encapsulates information about a server holding juju state and
// can be used to make a connection to it.
type Info struct {
	// Addrs holds the addresses of the state servers.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string

	// Tag holds the name of the entity that is connecting.
	// If this is nil, and the password are empty, no login attempt will be made.
	// (this is to allow tests to access the API to check that operations
	// fail when not logged in).
	Tag names.Tag

	// Password holds the password for the administrator or connecting entity.
	Password string

	// Nonce holds the nonce used when provisioning the machine. Used
	// only by the machine agent.
	Nonce string `yaml:",omitempty"`

	// EnvironTag holds the environ tag for the environment we are
	// trying to connect to.
	EnvironTag names.EnvironTag
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

// Open establishes a connection to the API server using the Info
// given, returning a State instance which can be used to make API
// requests.
//
// See Connect for details of the connection mechanics.
func Open(info *Info, opts DialOpts) (*State, error) {
	return open(info, opts, (*State).Login)
}

// This unexported open method is used both directly above in the Open
// function, and also the OpenWithVersion function below to explicitly cause
// the API server to think that the client is older than it really is.
func open(info *Info, opts DialOpts, loginFunc func(st *State, tag, pwd, nonce string) error) (*State, error) {
	conn, err := Connect(info, "", nil, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := rpc.NewConn(jsoncodec.NewWebsocket(conn), nil)
	client.Start()
	st := &State{
		client:     client,
		conn:       conn,
		addr:       conn.Config().Location.Host,
		serverRoot: "https://" + conn.Config().Location.Host,
		// why are the contents of the tag (username and password) written into the
		// state structure BEFORE login ?!?
		tag:      toString(info.Tag),
		password: info.Password,
		certPool: conn.Config().TlsConfig.RootCAs,
	}
	if info.Tag != nil || info.Password != "" {
		if err := loginFunc(st, info.Tag.String(), info.Password, info.Nonce); err != nil {
			conn.Close()
			return nil, err
		}
	}
	st.broken = make(chan struct{})
	st.closed = make(chan struct{})
	go st.heartbeatMonitor()
	return st, nil
}

// OpenWithVersion uses an explicit version of the Admin facade to call Login
// on. This allows the caller to pretend to be an older client, and is used
// only in testing.
func OpenWithVersion(info *Info, opts DialOpts, loginVersion int) (*State, error) {
	var loginFunc func(st *State, tag, pwd, nonce string) error
	switch loginVersion {
	case 0:
		loginFunc = (*State).loginV0
	case 1:
		loginFunc = (*State).loginV1
	case 2:
		loginFunc = (*State).loginV2
	default:
		return nil, errors.NotSupportedf("loginVersion %d", loginVersion)
	}
	return open(info, opts, loginFunc)
}

// Connect establishes a websocket connection to the API server using
// the Info, API path tail and (optional) request headers provided. If
// multiple API addresses are provided in Info they will be tried
// concurrently - the first successful connection wins.
//
// The path tail may be blank, in which case the default value will be
// used. Otherwise, it must start with a "/".
func Connect(info *Info, pathTail string, header http.Header, opts DialOpts) (*websocket.Conn, error) {
	if len(info.Addrs) == 0 {
		return nil, errors.New("no API addresses to connect to")
	}
	if pathTail != "" && !strings.HasPrefix(pathTail, "/") {
		return nil, errors.New(`path tail must start with "/"`)
	}

	pool := x509.NewCertPool()
	xcert, err := cert.ParseCert(info.CACert)
	if err != nil {
		return nil, errors.Annotate(err, "cert pool creation failed")
	}
	pool.AddCert(xcert)

	// If they exist, only use localhost addresses.
	var addrs []string
	for _, addr := range info.Addrs {
		if strings.HasPrefix(addr, "localhost:") {
			addrs = append(addrs, addr)
			break
		}
	}
	if len(addrs) == 0 {
		addrs = info.Addrs
	}

	path := makeAPIPath(info.EnvironTag.Id(), pathTail)

	// Dial all addresses at reasonable intervals.
	try := parallel.NewTry(0, nil)
	defer try.Kill()
	for _, addr := range addrs {
		err := dialWebsocket(addr, path, header, opts, pool, try)
		if err == parallel.ErrStopped {
			break
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		select {
		case <-time.After(opts.DialAddressInterval):
		case <-try.Dead():
		}
	}
	try.Close()
	result, err := try.Result()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conn := result.(*websocket.Conn)
	logger.Infof("connection established to %q", conn.RemoteAddr())
	return conn, nil
}

// makeAPIPath builds the path to connect to based on the tail given
// and whether the environment UUID is set.
func makeAPIPath(envUUID, tail string) string {
	if envUUID == "" {
		if tail == "" {
			tail = "/"
		}
		return tail
	}
	if tail == "" {
		tail = "/api"
	}
	return "/environment/" + envUUID + tail
}

// toString returns the value of a tag's String method, or "" if the tag is nil.
func toString(tag names.Tag) string {
	if tag == nil {
		return ""
	}
	return tag.String()
}

func dialWebsocket(addr, path string, header http.Header, opts DialOpts, rootCAs *x509.CertPool, try *parallel.Try) error {
	cfg, err := setUpWebsocket(addr, path, header, rootCAs)
	if err != nil {
		return err
	}
	return try.Start(newWebsocketDialer(cfg, opts))
}

func setUpWebsocket(addr, path string, header http.Header, rootCAs *x509.CertPool) (*websocket.Config, error) {
	// origin is required by the WebSocket API, used for "origin policy"
	// in websockets. We pass localhost to satisfy the API; it is
	// inconsequential to us.
	const origin = "http://localhost/"
	cfg, err := websocket.NewConfig("wss://"+addr+path, origin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg.TlsConfig = &tls.Config{
		RootCAs:    rootCAs,
		ServerName: "juju-apiserver",
	}
	cfg.Header = header
	return cfg, nil
}

// newWebsocketDialer returns a function that
// can be passed to utils/parallel.Try.Start.
var newWebsocketDialer = createWebsocketDialer

func createWebsocketDialer(cfg *websocket.Config, opts DialOpts) func(<-chan struct{}) (io.Closer, error) {
	openAttempt := utils.AttemptStrategy{
		Total: opts.Timeout,
		Delay: opts.RetryDelay,
	}
	return func(stop <-chan struct{}) (io.Closer, error) {
		for a := openAttempt.Start(); a.Next(); {
			select {
			case <-stop:
				return nil, parallel.ErrStopped
			default:
			}
			logger.Infof("dialing %q", cfg.Location)
			conn, err := websocket.DialConfig(cfg)
			if err == nil {
				return conn, nil
			}
			if a.HasNext() {
				logger.Debugf("error dialing %q, will retry: %v", cfg.Location, err)
			} else {
				logger.Infof("error dialing %q: %v", cfg.Location, err)
				return nil, errors.Errorf("unable to connect to %q", cfg.Location)
			}
		}
		panic("unreachable")
	}
}

func (s *State) heartbeatMonitor() {
	for {
		if err := s.Ping(); err != nil {
			close(s.broken)
			return
		}
		select {
		case <-time.After(PingPeriod):
		case <-s.closed:
		}
	}
}

func (s *State) Ping() error {
	return s.APICall("Pinger", s.BestFacadeVersion("Pinger"), "", "Ping", nil, nil)
}

// APICall places a call to the remote machine.
//
// This fills out the rpc.Request on the given facade, version for a given
// object id, and the specific RPC method. It marshalls the Arguments, and will
// unmarshall the result into the response object that is supplied.
func (s *State) APICall(facade string, version int, id, method string, args, response interface{}) error {
	err := s.client.Call(rpc.Request{
		Type:    facade,
		Version: version,
		Id:      id,
		Action:  method,
	}, args, response)
	return params.ClientError(err)
}

func (s *State) Close() error {
	err := s.client.Close()
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	<-s.broken
	return err
}

// Broken returns a channel that's closed when the connection is broken.
func (s *State) Broken() <-chan struct{} {
	return s.broken
}

// RPCClient returns the RPC client for the state, so that testing
// functions can tickle parts of the API that the conventional entry
// points don't reach. This is exported for testing purposes only.
func (s *State) RPCClient() *rpc.Conn {
	return s.client
}

// Addr returns the address used to connect to the API server.
func (s *State) Addr() string {
	return s.addr
}

// EnvironTag returns the tag of the environment we are connected to.
func (s *State) EnvironTag() (names.EnvironTag, error) {
	return names.ParseEnvironTag(s.environTag)
}

// ServerTag returns the tag of the server we are connected to.
func (s *State) ServerTag() (names.EnvironTag, error) {
	return names.ParseEnvironTag(s.serverTag)
}

// APIHostPorts returns addresses that may be used to connect
// to the API server, including the address used to connect.
//
// The addresses are scoped (public, cloud-internal, etc.), so
// the client may choose which addresses to attempt. For the
// Juju CLI, all addresses must be attempted, as the CLI may
// be invoked both within and outside the environment (think
// private clouds).
func (s *State) APIHostPorts() [][]network.HostPort {
	// NOTE: We're making a copy of s.hostPorts before returning it,
	// for safety.
	hostPorts := make([][]network.HostPort, len(s.hostPorts))
	for i, server := range s.hostPorts {
		hostPorts[i] = append([]network.HostPort{}, server...)
	}
	return hostPorts
}

// AllFacadeVersions returns what versions we know about for all facades
func (s *State) AllFacadeVersions() map[string][]int {
	facades := make(map[string][]int, len(s.facadeVersions))
	for name, versions := range s.facadeVersions {
		facades[name] = append([]int{}, versions...)
	}
	return facades
}

// BestFacadeVersion compares the versions of facades that we know about, and
// the versions available from the server, and reports back what version is the
// 'best available' to use.
// TODO(jam) this is the eventual implementation of what version of a given
// Facade we will want to use. It needs to line up the versions that the server
// reports to us, with the versions that our client knows how to use.
func (s *State) BestFacadeVersion(facade string) int {
	return bestVersion(facadeVersions[facade], s.facadeVersions[facade])
}
