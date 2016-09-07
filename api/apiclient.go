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
	"github.com/juju/retry"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/parallel"
	"github.com/juju/version"
	"golang.org/x/net/websocket"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
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

type rpcConnection interface {
	Call(req rpc.Request, params, response interface{}) error
	Close() error
}

// state is the internal implementation of the Connection interface.
type state struct {
	client rpcConnection
	conn   *websocket.Conn
	clock  clock.Clock

	// addr is the address used to connect to the API server.
	addr string

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
	// which the client may cache and use for failover.
	hostPorts [][]network.HostPort

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

	// tag, password, macaroons and nonce hold the cached login
	// credentials. These are only valid if loggedIn is 1.
	tag       string
	password  string
	macaroons []macaroon.Slice
	nonce     string

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

// RedirectError is returned from Open when the controller
// needs to inform the client that the model is hosted
// on a different set of API addresses.
type RedirectError struct {
	// Servers holds the sets of addresses of the redirected
	// servers.
	Servers [][]network.HostPort

	// CACert holds the certificate of the remote server.
	CACert string
}

func (e *RedirectError) Error() string {
	return fmt.Sprintf("redirection to alternative server required")
}

// Open establishes a connection to the API server using the Info
// given, returning a State instance which can be used to make API
// requests.
//
// If the model is hosted on a different server, Open
// will return an error with a *RedirectError cause
// holding the details of another server to connect to.
//
// See Connect for details of the connection mechanics.
func Open(info *Info, opts DialOpts) (Connection, error) {
	return open(info, opts, clock.WallClock)
}

// open is the unexported version of open that also includes
// an explicit clock instance argument.
func open(
	info *Info,
	opts DialOpts,
	clock clock.Clock,
) (Connection, error) {
	if err := info.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating info for opening an API connection")
	}
	if clock == nil {
		return nil, errors.NotValidf("nil clock")
	}
	conn, tlsConfig, err := connectWebsocket(info, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := rpc.NewConn(jsoncodec.NewWebsocket(conn), observer.None())
	client.Start()

	bakeryClient := opts.BakeryClient
	if bakeryClient == nil {
		bakeryClient = httpbakery.NewClient()
	} else {
		// Make a copy of the bakery client and its HTTP client
		c := *opts.BakeryClient
		bakeryClient = &c
		httpc := *bakeryClient.Client
		bakeryClient.Client = &httpc
	}
	apiHost := conn.Config().Location.Host
	// Technically when there's no CACert, we don't need this
	// machinery, because we could just use http.DefaultTransport
	// for everything, but it's easier just to leave it in place.
	bakeryClient.Client.Transport = &hostSwitchingTransport{
		primaryHost: apiHost,
		primary:     utils.NewHttpTLSTransport(tlsConfig),
		fallback:    http.DefaultTransport,
	}

	st := &state{
		client: client,
		conn:   conn,
		clock:  clock,
		addr:   apiHost,
		cookieURL: &url.URL{
			Scheme: "https",
			Host:   conn.Config().Location.Host,
			Path:   "/",
		},
		pingerFacadeVersion: facadeVersions["Pinger"],
		serverScheme:        "https",
		serverRootAddress:   conn.Config().Location.Host,
		// We populate the username and password before
		// login because, when doing HTTP requests, we'll want
		// to use the same username and password for authenticating
		// those. If login fails, we discard the connection.
		tag:          tagToString(info.Tag),
		password:     info.Password,
		macaroons:    info.Macaroons,
		nonce:        info.Nonce,
		tlsConfig:    tlsConfig,
		bakeryClient: bakeryClient,
		modelTag:     info.ModelTag,
	}
	if !info.SkipLogin {
		if err := st.Login(info.Tag, info.Password, info.Nonce, info.Macaroons); err != nil {
			conn.Close()
			return nil, errors.Trace(err)
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
	tlsConfig := utils.SecureTLSConfig()
	tlsConfig.InsecureSkipVerify = opts.InsecureSkipVerify

	if info.CACert != "" && !tlsConfig.InsecureSkipVerify {
		// We want to be specific here (rather than just using "anything".
		// See commit 7fc118f015d8480dfad7831788e4b8c0432205e8 (PR 899).
		tlsConfig.ServerName = "juju-apiserver"
		certPool, err := CreateCertPool(info.CACert)
		if err != nil {
			return nil, nil, errors.Annotate(err, "cert pool creation failed")
		}
		tlsConfig.RootCAs = certPool
	}
	path, err := apiPath(info.ModelTag, "/api")
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	conn, err := dialWebSocket(info.Addrs, path, tlsConfig, opts)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	logger.Infof("connection established to %q", conn.RemoteAddr())
	return conn, tlsConfig, nil
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

// ConnectStream implements StreamConnector.ConnectStream.
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
	path, err := apiPath(st.modelTag, path)
	if err != nil {
		return nil, errors.Trace(err)
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
// endpoint path and query parameters.
func (st *state) apiEndpoint(path, query string) (*url.URL, error) {
	path, err := apiPath(st.modelTag, path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &url.URL{
		Scheme:   st.serverScheme,
		Host:     st.Addr(),
		Path:     path,
		RawQuery: query,
	}, nil
}

// apiPath returns the given API endpoint path relative
// to the given model tag.
func apiPath(modelTag names.ModelTag, path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", errors.Errorf("cannot make API path from non-slash-prefixed path %q", path)
	}
	modelUUID := modelTag.Id()
	if modelUUID == "" {
		return path, nil
	}
	return "/model/" + modelUUID + path, nil
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
	// TODO(katco): 2016-08-09: lp:1611427
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
			if !a.HasNext() || isX509Error(err) {
				// We won't reconnect when there's an X509 error
				// because we're not going to succeed if we retry
				// in that case.
				logger.Infof("error dialing %q: %v", cfg.Location, err)
				return nil, errors.Annotatef(err, "unable to connect to API")
			}
		}
		panic("unreachable")
	}
}

// isX509Error reports whether the given websocket error
// results from an X509 problem.
func isX509Error(err error) bool {
	wsErr, ok := errors.Cause(err).(*websocket.DialError)
	if !ok {
		return false
	}
	switch wsErr.Err.(type) {
	case x509.HostnameError,
		x509.InsecureAlgorithmError,
		x509.UnhandledCriticalExtension,
		x509.UnknownAuthorityError,
		x509.ConstraintViolationError,
		x509.SystemRootsError:
		return true
	}
	switch err {
	case x509.ErrUnsupportedAlgorithm,
		x509.IncorrectPasswordError:
		return true
	}
	return false
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
	return s.APICall("Pinger", s.pingerFacadeVersion, "", "Ping", nil, nil)
}

type hasErrorCode interface {
	ErrorCode() string
}

// APICall places a call to the remote machine.
//
// This fills out the rpc.Request on the given facade, version for a given
// object id, and the specific RPC method. It marshalls the Arguments, and will
// unmarshall the result into the response object that is supplied.
func (s *state) APICall(facade string, version int, id, method string, args, response interface{}) error {
	retrySpec := retry.CallArgs{
		Func: func() error {
			return s.client.Call(rpc.Request{
				Type:    facade,
				Version: version,
				Id:      id,
				Action:  method,
			}, args, response)
		},
		IsFatalError: func(err error) bool {
			err = errors.Cause(err)
			ec, ok := err.(hasErrorCode)
			if !ok {
				return true
			}
			return ec.ErrorCode() != params.CodeRetry
		},
		Delay:       100 * time.Millisecond,
		MaxDelay:    1500 * time.Millisecond,
		MaxDuration: 10 * time.Second,
		BackoffFunc: retry.DoubleDelay,
		Clock:       s.clock,
	}
	err := retry.Call(retrySpec)
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

// Addr returns the address used to connect to the API server.
func (s *state) Addr() string {
	return s.addr
}

// ModelTag implements base.APICaller.ModelTag.
func (s *state) ModelTag() (names.ModelTag, bool) {
	return s.modelTag, s.modelTag.Id() != ""
}

// ControllerTag implements base.APICaller.ControllerTag.
func (s *state) ControllerTag() names.ControllerTag {
	return s.controllerTag
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
