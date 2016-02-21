// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/parallel"
	"golang.org/x/net/websocket"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.api")

// TODO(fwereade): we should be injecting a Clock; and injecting these values;
// across the board, instead of using these global variables.
var (
	// PingPeriod defines how often the internal connection health check
	// will run.
	PingPeriod = 1 * time.Minute

	// PingTimeout defines how long a health check can take before we
	// consider it to have failed.
	PingTimeout = 30 * time.Second
)

// state is the internal implementation of the Connection interface.
type state struct {
	client *rpc.Conn
	conn   *websocket.Conn

	// addr is the address used to connect to the API server.
	addr string

	// cookieURL is the URL that HTTP cookies for the API
	// will be associated with (specifically macaroon auth cookies).
	cookieURL *url.URL

	// modelTag holds the model tag once we're connected
	modelTag string

	// controllerTag holds the controller tag once we're connected.
	// This is only set with newer apiservers where they are using
	// the v1 login mechansim.
	controllerTag string

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

	// loggedIn holds whether the client has successfully logged
	// in. It's a int32 so that the atomic package can be used to
	// access it safely.
	loggedIn int32

	// tag and password and nonce hold the cached login credentials.
	// These are only valid if loggedIn is 1.
	tag      string
	password string
	nonce    string

	// serverRootAddress holds the cached API server address and port used
	// to login.
	serverRootAddress string

	// serverScheme is the URI scheme of the API Server
	serverScheme string

	// tlsConfig holds the TLS config appropriate for making SSL
	// connections to the API endpoints.
	tlsConfig *tls.Config

	// certPool holds the cert pool that is used to authenticate the tls
	// connections to the API.
	certPool *x509.CertPool

	// bakeryClient holds the client that will be used to
	// authorize macaroon based login requests.
	bakeryClient *httpbakery.Client
}

// Open establishes a connection to the API server using the Info
// given, returning a State instance which can be used to make API
// requests.
//
// See Connect for details of the connection mechanics.
func Open(info *Info, opts DialOpts) (Connection, error) {
	return open(info, opts, (*state).Login)
}

// This unexported open method is used both directly above in the Open
// function, and also the OpenWithVersion function below to explicitly cause
// the API server to think that the client is older than it really is.
func open(info *Info, opts DialOpts, loginFunc func(st *state, tag names.Tag, pwd, nonce string) error) (Connection, error) {
	if info.UseMacaroons {
		if info.Tag != nil || info.Password != "" {
			return nil, errors.New("open should specifiy UseMacaroons or a username & password. Not both")
		}
	}
	conn, tlsConfig, err := connectWebsocket(info, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := rpc.NewConn(jsoncodec.NewWebsocket(conn), nil)
	client.Start()

	bakeryClient := opts.BakeryClient
	if bakeryClient == nil {
		bakeryClient = httpbakery.NewClient()
	} else {
		// Make a copy of the bakery client and its
		// HTTP client
		c := *opts.BakeryClient
		bakeryClient = &c
		httpc := *bakeryClient.Client
		bakeryClient.Client = &httpc
	}
	apiHost := conn.Config().Location.Host
	bakeryClient.Client.Transport = &hostSwitchingTransport{
		primaryHost: apiHost,
		primary:     utils.NewHttpTLSTransport(tlsConfig),
		fallback:    http.DefaultTransport,
	}

	st := &state{
		client: client,
		conn:   conn,
		addr:   apiHost,
		cookieURL: &url.URL{
			Scheme: "https",
			Host:   conn.Config().Location.Host,
			Path:   "/",
		},
		serverScheme:      "https",
		serverRootAddress: conn.Config().Location.Host,
		// why are the contents of the tag (username and password) written into the
		// state structure BEFORE login ?!?
		tag:          tagToString(info.Tag),
		password:     info.Password,
		nonce:        info.Nonce,
		tlsConfig:    tlsConfig,
		bakeryClient: bakeryClient,
	}
	if info.Tag != nil || info.Password != "" || info.UseMacaroons {
		if err := loginFunc(st, info.Tag, info.Password, info.Nonce); err != nil {
			conn.Close()
			return nil, err
		}
	}
	st.broken = make(chan struct{})
	st.closed = make(chan struct{})
	go st.heartbeatMonitor()
	return st, nil
}

// hostSwitchingTransport provides an http.RoundTripper
// that chooses an actual RoundTripper to use
// depending on the destination host.
//
// This makes it possible to use a different set of root
// CAs for the API and all other hosts.
type hostSwitchingTransport struct {
	primaryHost string
	primary     http.RoundTripper
	fallback    http.RoundTripper
}

// RoundTrip implements http.RoundTripper.RoundTrip.
func (t *hostSwitchingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == t.primaryHost {
		return t.primary.RoundTrip(req)
	}
	return t.fallback.RoundTrip(req)
}

// OpenWithVersion uses an explicit version of the Admin facade to call Login
// on. This allows the caller to pretend to be an older client, and is used
// only in testing.
func OpenWithVersion(info *Info, opts DialOpts, loginVersion int) (Connection, error) {
	var loginFunc func(st *state, tag names.Tag, pwd, nonce string) error
	switch loginVersion {
	case 2:
		loginFunc = (*state).loginV2
	case 3:
		loginFunc = (*state).loginV3
	default:
		return nil, errors.NotSupportedf("loginVersion %d", loginVersion)
	}
	return open(info, opts, loginFunc)
}

// connectWebsocket establishes a websocket connection to the RPC
// API websocket on the API server using Info. If multiple API addresses
// are provided in Info they will be tried concurrently - the first successful
// connection wins.
//
// It also returns the TLS configuration that it has derived from the Info.
func connectWebsocket(info *Info, opts DialOpts) (*websocket.Conn, *tls.Config, error) {
	if len(info.Addrs) == 0 {
		return nil, nil, errors.New("no API addresses to connect to")
	}
	tlsConfig, err := tlsConfigForCACert(info.CACert)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot make TLS configuration")
	}
	tlsConfig.InsecureSkipVerify = opts.InsecureSkipVerify
	path := "/"
	if info.ModelTag.Id() != "" {
		path = apiPath(info.ModelTag, "/api")
	}
	conn, err := dialWebSocket(info.Addrs, path, tlsConfig, opts)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	logger.Infof("connection established to %q", conn.RemoteAddr())
	return conn, tlsConfig, nil
}

func tlsConfigForCACert(caCert string) (*tls.Config, error) {
	certPool, err := CreateCertPool(caCert)
	if err != nil {
		return nil, errors.Annotate(err, "cert pool creation failed")
	}
	return &tls.Config{
		RootCAs: certPool,
		// We want to be specific here (rather than just using "anything".
		// See commit 7fc118f015d8480dfad7831788e4b8c0432205e8 (PR 899).
		ServerName: "juju-apiserver",
	}, nil
}

// dialWebSocket dials a websocket with one of the provided addresses, the
// specified URL path, TLS configuration, and dial options. Each of the
// specified addresses will be attempted concurrently, and the first
// successful connection will be returned.
func dialWebSocket(addrs []string, path string, tlsConfig *tls.Config, opts DialOpts) (*websocket.Conn, error) {
	// Dial all addresses at reasonable intervals.
	try := parallel.NewTry(0, nil)
	defer try.Kill()
	for _, addr := range addrs {
		err := dialWebsocket(addr, path, opts, tlsConfig, try)
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
	return result.(*websocket.Conn), nil
}

// ConnectStream implements Connection.ConnectStream.
func (st *state) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
	if !st.isLoggedIn() {
		return nil, errors.New("cannot use ConnectStream without logging in")
	}
	// We use the standard "macaraq" macaroon authentication dance here.
	// That is, we attach any macaroons we have to the initial request,
	// and if that succeeds, all's good. If it fails with a DischargeRequired
	// error, the response will contain a macaroon that, when discharged,
	// may allow access, so we discharge it (using bakery.Client.HandleError)
	// and try the request again.
	conn, err := st.connectStream(path, attrs)
	if err == nil {
		return conn, err
	}
	if params.ErrCode(err) != params.CodeDischargeRequired {
		return nil, errors.Trace(err)
	}
	if err := st.bakeryClient.HandleError(st.cookieURL, bakeryError(err)); err != nil {
		return nil, errors.Trace(err)
	}
	// Try again with the discharged macaroon.
	conn, err = st.connectStream(path, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}

// connectStream is the internal version of ConnectStream. It differs from
// ConnectStream only in that it will not retry the connection if it encounters
// discharge-required error.
func (st *state) connectStream(path string, attrs url.Values) (base.Stream, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New(`path must start with "/"`)
	}
	if _, ok := st.ServerVersion(); ok {
		// If the server version is set, then we know the server is capable of
		// serving streams at the model path. We also fully expect
		// that the server has returned a valid model tag.
		modelTag, err := st.ModelTag()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get model tag, perhaps connected to system not model")
		}
		path = apiPath(modelTag, path)
	}
	target := url.URL{
		Scheme:   "wss",
		Host:     st.addr,
		Path:     path,
		RawQuery: attrs.Encode(),
	}
	cfg, err := websocket.NewConfig(target.String(), "http://localhost/")
	if st.tag != "" {
		cfg.Header = utils.BasicAuthHeader(st.tag, st.password)
	}
	if st.nonce != "" {
		cfg.Header.Set(params.MachineNonceHeader, st.nonce)
	}
	// Add any cookies because they will not be sent to websocket
	// connections by default.
	st.addCookiesToHeader(cfg.Header)

	cfg.TlsConfig = st.tlsConfig
	connection, err := websocketDialConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := readInitialStreamError(connection); err != nil {
		return nil, errors.Trace(err)
	}
	return connection, nil
}

// readInitialStreamError reads the initial error response
// from a stream connection and returns it.
func readInitialStreamError(conn io.Reader) error {
	// We can use bufio here because the websocket guarantees that a
	// single read will not read more than a single frame; there is
	// no guarantee that a single read might not read less than the
	// whole frame though, so using a single Read call is not
	// correct. By using ReadSlice rather than ReadBytes, we
	// guarantee that the error can't be too big (>4096 bytes).
	line, err := bufio.NewReader(conn).ReadSlice('\n')
	if err != nil {
		return errors.Annotate(err, "unable to read initial response")
	}
	var errResult params.ErrorResult
	if err := json.Unmarshal(line, &errResult); err != nil {
		return errors.Annotate(err, "unable to unmarshal initial response")
	}
	if errResult.Error != nil {
		return errResult.Error
	}
	return nil
}

// addCookiesToHeader adds any cookies associated with the
// API host to the given header. This is necessary because
// otherwise cookies are not sent to websocket endpoints.
func (st *state) addCookiesToHeader(h http.Header) {
	// net/http only allows adding cookies to a request,
	// but when it sends a request to a non-http endpoint,
	// it doesn't add the cookies, so make a request, starting
	// with the given header, add the cookies to use, then
	// throw away the request but keep the header.
	req := &http.Request{
		Header: h,
	}
	cookies := st.bakeryClient.Client.Jar.Cookies(st.cookieURL)
	for _, c := range cookies {
		req.AddCookie(c)
	}
}

// apiEndpoint returns a URL that refers to the given API slash-prefixed
// endpoint path and query parameters. Note that the caller
// is responsible for ensuring that the path *is* prefixed with a slash.
func (st *state) apiEndpoint(path, query string) (*url.URL, error) {
	if _, err := st.ControllerTag(); err == nil {
		// The controller tag is set, so the agent version is >= 1.23,
		// so we can use the model endpoint.
		modelTag, err := st.ModelTag()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get API endpoint address")
		}
		path = apiPath(modelTag, path)
	}
	return &url.URL{
		Scheme:   st.serverScheme,
		Host:     st.Addr(),
		Path:     path,
		RawQuery: query,
	}, nil
}

// apiPath returns the given API endpoint path relative
// to the given model tag. The caller is responsible
// for ensuring that the model tag is valid and
// that the path is slash-prefixed.
func apiPath(modelTag names.ModelTag, path string) string {
	if !strings.HasPrefix(path, "/") {
		panic(fmt.Sprintf("apiPath called with non-slash-prefixed path %q", path))
	}
	if modelTag.Id() == "" {
		panic("apiPath called with empty model tag")
	}
	if modelUUID := modelTag.Id(); modelUUID != "" {
		return "/model/" + modelUUID + path
	}
	return path
}

// tagToString returns the value of a tag's String method, or "" if the tag is nil.
func tagToString(tag names.Tag) string {
	if tag == nil {
		return ""
	}
	return tag.String()
}

func dialWebsocket(addr, path string, opts DialOpts, tlsConfig *tls.Config, try *parallel.Try) error {
	// origin is required by the WebSocket API, used for "origin policy"
	// in websockets. We pass localhost to satisfy the API; it is
	// inconsequential to us.
	const origin = "http://localhost/"
	cfg, err := websocket.NewConfig("wss://"+addr+path, origin)
	if err != nil {
		return errors.Trace(err)
	}
	cfg.TlsConfig = tlsConfig
	return try.Start(newWebsocketDialer(cfg, opts))
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
				return nil, errors.Annotatef(err, "unable to connect to API")
			}
		}
		panic("unreachable")
	}
}

func callWithTimeout(f func() error, timeout time.Duration) bool {
	result := make(chan error, 1)
	go func() {
		// Note that result is buffered so that we don't leak this
		// goroutine when a timeout happens.
		result <- f()
	}()
	select {
	case err := <-result:
		if err != nil {
			logger.Debugf("health ping failed: %v", err)
		}
		return err == nil
	case <-time.After(timeout):
		logger.Errorf("health ping timed out after %s", timeout)
		return false
	}
}

func (s *state) heartbeatMonitor() {
	for {
		if !callWithTimeout(s.Ping, PingTimeout) {
			close(s.broken)
			return
		}
		select {
		case <-time.After(PingPeriod):
		case <-s.closed:
		}
	}
}

func (s *state) Ping() error {
	return s.APICall("Pinger", s.BestFacadeVersion("Pinger"), "", "Ping", nil, nil)
}

// APICall places a call to the remote machine.
//
// This fills out the rpc.Request on the given facade, version for a given
// object id, and the specific RPC method. It marshalls the Arguments, and will
// unmarshall the result into the response object that is supplied.
func (s *state) APICall(facade string, version int, id, method string, args, response interface{}) error {
	err := s.client.Call(rpc.Request{
		Type:    facade,
		Version: version,
		Id:      id,
		Action:  method,
	}, args, response)
	return errors.Trace(err)
}

func (s *state) Close() error {
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
func (s *state) Broken() <-chan struct{} {
	return s.broken
}

// RPCClient returns the RPC client for the state, so that testing
// functions can tickle parts of the API that the conventional entry
// points don't reach. This is exported for testing purposes only.
func (s *state) RPCClient() *rpc.Conn {
	return s.client
}

// Addr returns the address used to connect to the API server.
func (s *state) Addr() string {
	return s.addr
}

// ModelTag returns the tag of the model we are connected to.
func (s *state) ModelTag() (names.ModelTag, error) {
	return names.ParseModelTag(s.modelTag)
}

// ControllerTag returns the tag of the server we are connected to.
func (s *state) ControllerTag() (names.ModelTag, error) {
	return names.ParseModelTag(s.controllerTag)
}

// APIHostPorts returns addresses that may be used to connect
// to the API server, including the address used to connect.
//
// The addresses are scoped (public, cloud-internal, etc.), so
// the client may choose which addresses to attempt. For the
// Juju CLI, all addresses must be attempted, as the CLI may
// be invoked both within and outside the model (think
// private clouds).
func (s *state) APIHostPorts() [][]network.HostPort {
	// NOTE: We're making a copy of s.hostPorts before returning it,
	// for safety.
	hostPorts := make([][]network.HostPort, len(s.hostPorts))
	for i, server := range s.hostPorts {
		hostPorts[i] = append([]network.HostPort{}, server...)
	}
	return hostPorts
}

// AllFacadeVersions returns what versions we know about for all facades
func (s *state) AllFacadeVersions() map[string][]int {
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
func (s *state) BestFacadeVersion(facade string) int {
	return bestVersion(facadeVersions[facade], s.facadeVersions[facade])
}

// serverRoot returns the cached API server address and port used
// to login, prefixed with "<URI scheme>://" (usually https).
func (s *state) serverRoot() string {
	return s.serverScheme + "://" + s.serverRootAddress
}

func (s *state) isLoggedIn() bool {
	return atomic.LoadInt32(&s.loggedIn) == 1
}

func (s *state) setLoggedIn() {
	atomic.StoreInt32(&s.loggedIn, 1)
}
