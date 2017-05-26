// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/parallel"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/retry.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/utils/proxy"
)

// PingPeriod defines how often the internal connection health check
// will run.
const PingPeriod = 1 * time.Minute

// pingTimeout defines how long a health check can take before we
// consider it to have failed.
const pingTimeout = 30 * time.Second

// modelRoot is the prefix that all model API paths begin with.
const modelRoot = "/model/"

// Use a 64k frame size for the websockets while we need to deal
// with x/net/websocket connections that don't deal with recieving
// fragmented messages.
const websocketFrameSize = 65536

var logger = loggo.GetLogger("juju.api")

type rpcConnection interface {
	Call(req rpc.Request, params, response interface{}) error
	Dead() <-chan struct{}
	Close() error
}

// state is the internal implementation of the Connection interface.
type state struct {
	client rpcConnection
	conn   jsoncodec.JSONConn
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
	if err := info.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating info for opening an API connection")
	}
	if opts.Clock == nil {
		opts.Clock = clock.WallClock
	}
	dialResult, err := dialAPI(info, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := rpc.NewConn(jsoncodec.New(dialResult.conn), observer.None())
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
	apiURL, err := url.Parse(dialResult.urlStr)
	if err != nil {
		// This should never happen as the url would have failed during dialAPI above.
		// However the code paths don't allow capture of the url.URL used.
		return nil, errors.Trace(err)
	}
	apiHost := apiURL.Host

	// Technically when there's no CACert, we don't need this
	// machinery, because we could just use http.DefaultTransport
	// for everything, but it's easier just to leave it in place.
	bakeryClient.Client.Transport = &hostSwitchingTransport{
		primaryHost: apiHost,
		primary:     utils.NewHttpTLSTransport(dialResult.tlsConfig),
		fallback:    http.DefaultTransport,
	}

	st := &state{
		client: client,
		conn:   dialResult.conn,
		clock:  opts.Clock,
		addr:   apiHost,
		cookieURL: &url.URL{
			Scheme: "https",
			Host:   apiHost,
			Path:   "/",
		},
		pingerFacadeVersion: facadeVersions["Pinger"],
		serverScheme:        "https",
		serverRootAddress:   apiHost,
		// We populate the username and password before
		// login because, when doing HTTP requests, we'll want
		// to use the same username and password for authenticating
		// those. If login fails, we discard the connection.
		tag:          tagToString(info.Tag),
		password:     info.Password,
		macaroons:    info.Macaroons,
		nonce:        info.Nonce,
		tlsConfig:    dialResult.tlsConfig,
		bakeryClient: bakeryClient,
		modelTag:     info.ModelTag,
	}
	if !info.SkipLogin {
		if err := st.Login(info.Tag, info.Password, info.Nonce, info.Macaroons); err != nil {
			dialResult.conn.Close()
			return nil, errors.Trace(err)
		}
	}

	st.broken = make(chan struct{})
	st.closed = make(chan struct{})

	go (&monitor{
		clock:       opts.Clock,
		ping:        st.Ping,
		pingPeriod:  PingPeriod,
		pingTimeout: pingTimeout,
		closed:      st.closed,
		dead:        client.Dead(),
		broken:      st.broken,
	}).run()
	return st, nil
}

type NewConnectionForModelFunc func(*Info) (func(string) (Connection, error), error)

// NewConnectionForModel returns a function which returns a model API
// connection for a specified model UUID, based on the specified api info.
// Currently, such a connection will always be to a single controller.
func NewConnectionForModel(apiInfo *Info) (func(string) (Connection, error), error) {
	return func(modelUUID string) (Connection, error) {
		apiInfo.ModelTag = names.NewModelTag(modelUUID)
		conn, err := Open(apiInfo, DialOpts{
			Timeout:    time.Second,
			RetryDelay: 200 * time.Millisecond,
		})
		if err != nil {
			return nil, errors.Annotatef(err, "failed to open API to model %s", modelUUID)
		}
		return conn, nil
	}, nil
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

// ConnectStream implements StreamConnector.ConnectStream.
func (st *state) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
	path, err := apiPath(st.modelTag, path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conn, err := st.connectStreamWithRetry(path, attrs, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}

// ConnectControllerStream creates a stream connection to an API path
// that isn't prefixed with /model/uuid.
func (st *state) ConnectControllerStream(path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.Errorf("path %q is not absolute", path)
	}
	if strings.HasPrefix(path, modelRoot) {
		return nil, errors.Errorf("path %q is model-specific", path)
	}
	conn, err := st.connectStreamWithRetry(path, attrs, headers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}

func (st *state) connectStreamWithRetry(path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	if !st.isLoggedIn() {
		return nil, errors.New("cannot use ConnectStream without logging in")
	}
	// We use the standard "macaraq" macaroon authentication dance here.
	// That is, we attach any macaroons we have to the initial request,
	// and if that succeeds, all's good. If it fails with a DischargeRequired
	// error, the response will contain a macaroon that, when discharged,
	// may allow access, so we discharge it (using bakery.Client.HandleError)
	// and try the request again.
	conn, err := st.connectStream(path, attrs, headers)
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
	conn, err = st.connectStream(path, attrs, headers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}

// connectStream is the internal version of ConnectStream. It differs from
// ConnectStream only in that it will not retry the connection if it encounters
// discharge-required error.
func (st *state) connectStream(path string, attrs url.Values, extraHeaders http.Header) (base.Stream, error) {
	target := url.URL{
		Scheme:   "wss",
		Host:     st.addr,
		Path:     path,
		RawQuery: attrs.Encode(),
	}
	// TODO(macgreagoir) IPv6. Ubuntu still always provides IPv4 loopback,
	// and when/if this changes localhost should resolve to IPv6 loopback
	// in any case (lp:1644009). Review.

	dialer := &websocket.Dialer{
		Proxy:           proxy.DefaultConfig.GetProxy,
		TLSClientConfig: st.tlsConfig,
		// In order to deal with the remote side not handling message
		// fragmentation, we default to largeish frames.
		ReadBufferSize:  websocketFrameSize,
		WriteBufferSize: websocketFrameSize,
	}
	var requestHeader http.Header
	if st.tag != "" {
		requestHeader = utils.BasicAuthHeader(st.tag, st.password)
	} else {
		requestHeader = make(http.Header)
	}
	requestHeader.Set("Origin", "http://localhost/")
	if st.nonce != "" {
		requestHeader.Set(params.MachineNonceHeader, st.nonce)
	}
	// Add any cookies because they will not be sent to websocket
	// connections by default.
	err := st.addCookiesToHeader(requestHeader)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for header, values := range extraHeaders {
		for _, value := range values {
			requestHeader.Add(header, value)
		}
	}

	connection, err := websocketDial(dialer, target.String(), requestHeader)
	if err != nil {
		return nil, err
	}
	if err := readInitialStreamError(connection); err != nil {
		connection.Close()
		return nil, errors.Trace(err)
	}
	return connection, nil
}

// readInitialStreamError reads the initial error response
// from a stream connection and returns it.
func readInitialStreamError(ws base.Stream) error {
	// We can use bufio here because the websocket guarantees that a
	// single read will not read more than a single frame; there is
	// no guarantee that a single read might not read less than the
	// whole frame though, so using a single Read call is not
	// correct. By using ReadSlice rather than ReadBytes, we
	// guarantee that the error can't be too big (>4096 bytes).
	messageType, reader, err := ws.NextReader()
	if err != nil {
		return errors.Annotate(err, "unable to get reader")
	}
	if messageType != websocket.TextMessage {
		return errors.Errorf("unexpected message type %v", messageType)
	}
	line, err := bufio.NewReader(reader).ReadSlice('\n')
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
func (st *state) addCookiesToHeader(h http.Header) error {
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
	if len(cookies) == 0 && len(st.macaroons) > 0 {
		// These macaroons must have been added directly rather than
		// obtained from a request. Add them. (For example in the
		// logtransfer connection for a migration.)
		// See https://bugs.launchpad.net/juju/+bug/1650451
		for _, macaroon := range st.macaroons {
			cookie, err := httpbakery.NewCookie(macaroon)
			if err != nil {
				return errors.Trace(err)
			}
			req.AddCookie(cookie)
		}
	}
	return nil
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

// Ping implements api.Connection.
func (s *state) Ping() error {
	return s.APICall("Pinger", s.pingerFacadeVersion, "", "Ping", nil, nil)
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
	return modelRoot + modelUUID + path, nil
}

// tagToString returns the value of a tag's String method, or "" if the tag is nil.
func tagToString(tag names.Tag) string {
	if tag == nil {
		return ""
	}
	return tag.String()
}

// dialResult holds a dialled connection, the URL
// and TLS configuration used to connect to it.
type dialResult struct {
	conn      jsoncodec.JSONConn
	urlStr    string
	tlsConfig *tls.Config
}

// Close implements io.Closer by closing the websocket
// connection. It is implemented so that a *dialResult
// value can be used as the result of a parallel.Try.
func (c *dialResult) Close() error {
	return c.conn.Close()
}

// dialOpts holds the original dial options
// but adds some information for the local dial logic.
type dialOpts struct {
	DialOpts
	tlsConfig   *tls.Config
	sniHostName string
	deadline    time.Time
}

// dialAPI establishes a websocket connection to the RPC
// API websocket on the API server using Info. If multiple API addresses
// are provided in Info they will be tried concurrently - the first successful
// connection wins.
//
// It also returns the TLS configuration that it has derived from the Info.
func dialAPI(info *Info, opts0 DialOpts) (*dialResult, error) {
	if len(info.Addrs) == 0 {
		return nil, errors.New("no API addresses to connect to")
	}
	opts := dialOpts{
		DialOpts:    opts0,
		sniHostName: info.SNIHostName,
	}
	tlsConfig := utils.SecureTLSConfig()
	tlsConfig.InsecureSkipVerify = opts.InsecureSkipVerify
	if info.CACert != "" {
		// We want to be specific here (rather than just using "anything".
		// See commit 7fc118f015d8480dfad7831788e4b8c0432205e8 (PR 899).
		tlsConfig.ServerName = "juju-apiserver"
		certPool, err := CreateCertPool(info.CACert)
		if err != nil {
			return nil, errors.Annotate(err, "cert pool creation failed")
		}
		tlsConfig.RootCAs = certPool
	} else {
		// No CA certificate so use the SNI host name for all
		// connections (if SNIHostName is empty, the host
		// name in the address will be used as usual).
		tlsConfig.ServerName = info.SNIHostName
	}
	opts.tlsConfig = tlsConfig
	// Set opts.DialWebsocket and opts.Clock here rather than in open because
	// some tests call dialAPI directly.
	if opts.DialWebsocket == nil {
		opts.DialWebsocket = gorillaDialWebsocket
	}
	if opts.Clock == nil {
		opts.Clock = clock.WallClock
	}
	// TODO(rogpeppe) Pass a context with an existing deadline into dialAPI.
	// We're avoiding that for the moment because it will break
	// lots of tests that rely on passing a zero timeout into
	// DialOpts.
	ctx := context.TODO()
	opts.deadline = opts.Clock.Now().Add(opts.Timeout)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	path, err := apiPath(info.ModelTag, "/api")
	if err != nil {
		return nil, errors.Trace(err)
	}
	dialInfo, err := dialWebsocketMulti(ctx, info.Addrs, path, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("connection established to %q", dialInfo.urlStr)
	return dialInfo, nil
}

// gorillaDialWebsocket makes a websocket connection using the
// gorilla websocket package.
func gorillaDialWebsocket(ctx context.Context, urlStr string, tlsConfig *tls.Config) (jsoncodec.JSONConn, error) {
	// TODO(rogpeppe) We'd like to set Deadline here
	// but that would break lots of tests that rely on
	// setting a zero timeout.
	netDialer := net.Dialer{}
	dialer := &websocket.Dialer{
		NetDial: func(netw, addr string) (net.Conn, error) {
			return netDialer.DialContext(ctx, netw, addr)
		},
		Proxy:           proxy.DefaultConfig.GetProxy,
		TLSClientConfig: tlsConfig,
		// In order to deal with the remote side not handling message
		// fragmentation, we default to largeish frames.
		ReadBufferSize:  websocketFrameSize,
		WriteBufferSize: websocketFrameSize,
	}
	// Note: no extra headers.
	c, _, err := dialer.Dial(urlStr, nil)
	if err != nil {
		return nil, err
	}
	return jsoncodec.NewWebsocketConn(c), nil
}

// dialWebsocketMulti dials a websocket with one of the provided addresses, the
// specified URL path, TLS configuration, and dial options. Each of the
// specified addresses will be attempted concurrently, and the first
// successful connection will be returned.
func dialWebsocketMulti(ctx context.Context, addrs []string, path string, opts dialOpts) (*dialResult, error) {
	// Dial all addresses at reasonable intervals.
	try := parallel.NewTry(0, nil)
	defer try.Kill()
	for _, addr := range addrs {
		err := startDialWebsocket(ctx, try, addr, path, opts)
		if err == parallel.ErrStopped {
			break
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		select {
		case <-opts.Clock.After(opts.DialAddressInterval):
		case <-try.Dead():
		}
	}
	try.Close()
	result, err := try.Result()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.(*dialResult), nil
}

// startDialWebsocket starts websocket connection to a single address
// on the given try instance.
func startDialWebsocket(ctx context.Context, try *parallel.Try, addr, path string, opts dialOpts) error {
	openAttempt := retry.Regular{
		Total: opts.Timeout,
		Delay: opts.RetryDelay,
	}
	if openAttempt.Min == 0 && openAttempt.Delay > 0 {
		openAttempt.Min = int(openAttempt.Total / openAttempt.Delay)
	}
	d := dialer{
		ctx:         ctx,
		openAttempt: openAttempt,
		addr:        addr,
		urlStr:      "wss://" + addr + path,
		opts:        opts,
	}
	return try.Start(d.dial)
}

type dialer struct {
	ctx         context.Context
	openAttempt retry.Strategy
	addr        string
	urlStr      string
	opts        dialOpts
}

// dial implements the function value expected by Try.Start
// by dialing the websocket as specified in d and retrying
// when appropriate.
func (d dialer) dial(_ <-chan struct{}) (io.Closer, error) {
	a := retry.StartWithCancel(d.openAttempt, d.opts.Clock, d.ctx.Done())
	for a.Next() {
		conn, tlsConfig, err := d.dial1()
		if err == nil {
			return &dialResult{
				conn:      conn,
				urlStr:    d.urlStr,
				tlsConfig: tlsConfig,
			}, nil
		}
		if isX509Error(err) || !a.More() {
			// certificate errors don't improve with retries.
			logger.Debugf("error dialing websocket: %v", err)
			return nil, errors.Annotatef(err, "unable to connect to API")
		}
	}
	return nil, parallel.ErrStopped
}

// dial1 makes a single dial attempt.
func (d dialer) dial1() (jsoncodec.JSONConn, *tls.Config, error) {
	conn, err := d.opts.DialWebsocket(d.ctx, d.urlStr, d.opts.tlsConfig)
	if err == nil {
		logger.Debugf("successfully dialed %q", d.urlStr)
		return conn, d.opts.tlsConfig, nil
	}
	if !isX509Error(err) {
		return nil, nil, errors.Trace(err)
	}
	if (d.opts.sniHostName == "" && isNumericIP(d.addr)) || d.opts.tlsConfig.RootCAs == nil {
		// We're trying to connect to a numeric IP address with
		// no other server name available, or we're using public
		// certificate validation. In the former case, using
		// public cert validation won't help, because you
		// generally can't obtain a public cert for a numeric IP
		// address. In the latter case, we either don't have the
		// private CA cert or we've already tried it. In both
		// those cases, we won't succeed when trying again
		// because a cert error isn't temporary, so return
		// immediately.
		//
		// Note that the error returned from
		// websocket.DialConfig always includes the location in
		// the message.
		return nil, nil, errors.Trace(err)
	}
	// It's possible we're inappropriately using the private
	// CA certificate, so retry immediately with the public one.
	tlsConfig1 := *d.opts.tlsConfig
	tlsConfig1.RootCAs = nil
	tlsConfig1.ServerName = d.opts.sniHostName
	conn, err = d.opts.DialWebsocket(d.ctx, d.urlStr, &tlsConfig1)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return conn, &tlsConfig1, nil
}

func isNumericIP(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return net.ParseIP(host) != nil
}

// isX509Error reports whether the given websocket error
// results from an X509 problem.
func isX509Error(err error) bool {
	switch errType := errors.Cause(err).(type) {
	case *websocket.CloseError:
		return errType.Code == websocket.CloseTLSHandshake
	case x509.CertificateInvalidError,
		x509.HostnameError,
		x509.InsecureAlgorithmError,
		x509.UnhandledCriticalExtension,
		x509.UnknownAuthorityError,
		x509.ConstraintViolationError,
		x509.SystemRootsError:
		return true
	default:
		return false
	}
}

var apiCallRetryStrategy = retry.LimitTime(10*time.Second,
	retry.Exponential{
		Initial:  100 * time.Millisecond,
		Factor:   2,
		MaxDelay: 1500 * time.Millisecond,
	},
)

// APICall places a call to the remote machine.
//
// This fills out the rpc.Request on the given facade, version for a given
// object id, and the specific RPC method. It marshalls the Arguments, and will
// unmarshall the result into the response object that is supplied.
func (s *state) APICall(facade string, version int, id, method string, args, response interface{}) error {
	for a := retry.Start(apiCallRetryStrategy, s.clock); a.Next(); {
		err := s.client.Call(rpc.Request{
			Type:    facade,
			Version: version,
			Id:      id,
			Action:  method,
		}, args, response)
		if params.ErrCode(err) != params.CodeRetry {
			return errors.Trace(err)
		}
		if !a.More() {
			return errors.Annotatef(err, "too many retries")
		}
	}
	panic("unreachable")
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

// Broken implements api.Connection.
func (s *state) Broken() <-chan struct{} {
	return s.broken
}

// IsBroken implements api.Connection.
func (s *state) IsBroken() bool {
	select {
	case <-s.broken:
		return true
	default:
	}
	if err := s.Ping(); err != nil {
		logger.Debugf("connection ping failed: %v", err)
		return true
	}
	return false
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

// PublicDNSName returns the host name for which an officially
// signed certificate will be used for TLS connection to the server.
// If empty, the private Juju CA certificate must be used to verify
// the connection.
func (s *state) PublicDNSName() string {
	return s.publicDNSName
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
