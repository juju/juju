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
	"math/rand"
	"net"
	"net/http"
	"net/url"
	gopath "path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/gorilla/websocket"
	"github.com/juju/clock"
	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/parallel"
	"gopkg.in/retry.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/network"
	jujuproxy "github.com/juju/juju/proxy"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/utils/proxy"
	jujuversion "github.com/juju/juju/version"
)

// PingPeriod defines how often the internal connection health check
// will run.
const PingPeriod = 1 * time.Minute

// pingTimeout defines how long a health check can take before we
// consider it to have failed.
const pingTimeout = 30 * time.Second

// modelRoot is the prefix that all model API paths begin with.
const modelRoot = "/model/"

// apiScheme is the default scheme used for connecting to the websocket API.
const apiScheme = "wss"

// serverScheme is the default scheme used for HTTP requests.
const serverScheme = "https"

var logger = loggo.GetLogger("juju.api")

type rpcConnection interface {
	Call(req rpc.Request, params, response interface{}) error
	Dead() <-chan struct{}
	Close() error
}

// RedirectError is returned from Open when the controller
// needs to inform the client that the model is hosted
// on a different set of API addresses.
type RedirectError struct {
	// Servers holds the sets of addresses of the redirected
	// servers.
	Servers []network.MachineHostPorts

	// CACert holds the certificate of the remote server.
	CACert string

	// FollowRedirect is set to true for cases like JAAS where the client
	// needs to automatically follow the redirect to the new controller.
	FollowRedirect bool

	// ControllerTag uniquely identifies the controller being redirected to.
	ControllerTag names.ControllerTag

	// An optional alias for the controller the model got redirected to.
	// It can be used by the client to present the user with a more
	// meaningful juju login -c XYZ command
	ControllerAlias string
}

func (e *RedirectError) Error() string {
	return "redirection to alternative server required"
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
	ctx := context.Background()
	dialCtx := ctx
	if opts.Timeout > 0 {
		ctx1, cancel := utils.ContextWithTimeout(dialCtx, opts.Clock, opts.Timeout)
		defer cancel()
		dialCtx = ctx1
	}

	dialResult, err := dialAPI(dialCtx, info, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := rpc.NewConn(jsoncodec.New(dialResult.conn), nil)
	client.Start(ctx)

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

	// Technically when there's no CACert, we don't need this
	// machinery, because we could just use http.DefaultTransport
	// for everything, but it's easier just to leave it in place.
	bakeryClient.Client.Transport = &hostSwitchingTransport{
		primaryHost: dialResult.controllerRootAddr.Host,
		primary: jujuhttp.NewHTTPTLSTransport(jujuhttp.TransportConfig{
			TLSConfig: dialResult.tlsConfig,
		}),
		fallback: http.DefaultTransport,
	}

	host := PreferredHost(info)
	if host == "" {
		host = dialResult.controllerRootAddr.Host
	}

	pingerFacadeVersions := facadeVersions["Pinger"]
	if len(pingerFacadeVersions) == 0 {
		return nil, errors.Errorf("pinger facade version is required")
	}

	loginProvider := opts.LoginProvider
	// TODO (alesstimec, wallyworld): login provider should be constructed outside
	// of this function and always passed in as part of dial opts. Also Info
	// does not need to hold the authentication related data anymore. Until that
	// is refactored we fall back to using the user-pass login provider
	// with information from Info.
	if loginProvider == nil {
		loginProvider = NewLegacyLoginProvider(info.Tag, info.Password, info.Nonce, info.Macaroons, CookieURLFromHost(host))
	}

	st := &state{
		ctx:                 context.Background(),
		client:              client,
		conn:                dialResult.conn,
		clock:               opts.Clock,
		addr:                dialResult.controllerRootAddr,
		ipAddr:              dialResult.ipAddr,
		cookieURL:           CookieURLFromHost(host),
		pingerFacadeVersion: pingerFacadeVersions[len(pingerFacadeVersions)-1],
		serverScheme:        serverScheme,
		// We keep the login provider around to provide auth headers
		// when doing HTTP requests.
		// If login fails, we discard the connection.
		loginProvider: loginProvider,
		tlsConfig:     dialResult.tlsConfig,
		bakeryClient:  bakeryClient,
		modelTag:      info.ModelTag,
		proxier:       dialResult.proxier,
	}
	if !info.SkipLogin {
		if err := loginWithContext(dialCtx, st, loginProvider); err != nil {
			dialResult.conn.Close()
			return nil, errors.Trace(err)
		}
	}

	st.broken = make(chan struct{})
	st.closed = make(chan struct{})

	go (&monitor{
		clock:       opts.Clock,
		ping:        st.ping,
		pingPeriod:  PingPeriod,
		pingTimeout: pingTimeout,
		closed:      st.closed,
		dead:        client.Dead(),
		broken:      st.broken,
	}).run()
	return st, nil
}

// CookieURLFromHost creates a url.URL from a given host.
func CookieURLFromHost(host string) *url.URL {
	return &url.URL{
		Scheme: serverScheme,
		Host:   host,
		Path:   "/",
	}
}

// PreferredHost returns the SNI hostname or controller name for the cookie URL
// so that it is stable when used with a HA controller cluster.
func PreferredHost(info *Info) string {
	if info == nil {
		return ""
	}

	host := info.SNIHostName
	if host == "" && info.ControllerUUID != "" {
		host = info.ControllerUUID
	}
	return host
}

// loginWithContext wraps st.Login with code that terminates
// if the context is cancelled.
// TODO(rogpeppe) pass Context into Login (and all API calls) so
// that this becomes unnecessary.
func loginWithContext(ctx context.Context, st *state, loginProvider LoginProvider) error {
	if loginProvider == nil {
		return errors.New("login provider not specified")
	}

	result := make(chan error, 1)
	go func() {
		loginResult, err := loginProvider.Login(ctx, st)
		if err != nil {
			result <- err
			return
		}

		result <- st.setLoginResult(loginResult)
	}()
	select {
	case err := <-result:
		return errors.Trace(err)
	case <-ctx.Done():
		return errors.Annotatef(ctx.Err(), "cannot log in")
	}
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

// Context returns the context associated with this state.
func (st *state) Context() context.Context {
	return st.ctx
}

// ConnectStream implements StreamConnector.ConnectStream. The stream
// returned will apply a 30-second write deadline, so WriteJSON should
// only be called from one goroutine.
func (st *state) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
	path, err := apiPath(st.modelTag.Id(), path)
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
// that isn't prefixed with /model/uuid - the target model (if the
// endpoint needs one) can be specified in the headers. The stream
// returned will apply a 30-second write deadline, so WriteJSON should
// only be called from one goroutine.
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
	if err := st.bakeryClient.HandleError(st.ctx, st.cookieURL, bakeryError(err)); err != nil {
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
	target := st.Addr()
	target.Path = gopath.Join(target.Path, path)
	target.RawQuery = attrs.Encode()

	// TODO(macgreagoir) IPv6. Ubuntu still always provides IPv4 loopback,
	// and when/if this changes localhost should resolve to IPv6 loopback
	// in any case (lp:1644009). Review.

	dialer := &websocket.Dialer{
		Proxy:           proxy.DefaultConfig.GetProxy,
		TLSClientConfig: st.tlsConfig,
	}
	requestHeader, err := st.loginProvider.AuthHeader()
	if err != nil {
		return nil, errors.Trace(err)
	}
	requestHeader.Set(params.JujuClientVersion, jujuversion.Current.String())
	for header, values := range extraHeaders {
		for _, value := range values {
			requestHeader.Add(header, value)
		}
	}

	connection, err := WebsocketDial(dialer, target.String(), requestHeader)
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

// ControllerAPIURL returns the URL to use to connect to the controller API.
// The address may contain a path component but may not contain a scheme
// e.g. https://
func ControllerAPIURL(addr string, port int) string {
	addr, path, _ := strings.Cut(addr, "/")
	hp := net.JoinHostPort(addr, strconv.Itoa(port))
	urlStr, _ := url.QueryUnescape(apiURL(hp, "", path).String())
	return urlStr
}

func apiURL(addr, model, path string) *url.URL {
	apiPath, _ := apiPath(model, "/api")
	path = gopath.Join(path, apiPath)
	return &url.URL{
		Scheme: apiScheme,
		Host:   addr,
		Path:   path,
	}
}

// ping implements calls the Pinger.ping facade.
func (s *state) ping() error {
	return s.APICall("Pinger", s.pingerFacadeVersion, "", "Ping", nil, nil)
}

// apiPath returns the given API endpoint path relative
// to the given model string.
func apiPath(model, path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", errors.Errorf("cannot make API path from non-slash-prefixed path %q", path)
	}
	if model == "" {
		return path, nil
	}
	return modelRoot + model + path, nil
}

// dialResult holds a dialed connection, the URL
// and TLS configuration used to connect to it.
type dialResult struct {
	conn jsoncodec.JSONConn
	// controllerRootAddr represents the controller's root address
	// e.g. wss://controller.com/foo
	controllerRootAddr *url.URL
	// dialAddr is the full URL (with no DNS resolution) that was actually dialed
	// e.g. wss://controller.com/foo/api for the controller endpoint
	// or wss://controller.com/foo/model/<uuid>/api for model endpoints.
	dialAddr *url.URL
	// ipAddr represents the IP address that was dialed.
	ipAddr    string
	proxier   jujuproxy.Proxier
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
	sniHostName string
	// certPool holds a cert pool containing the CACert
	// if there is one.
	certPool *x509.CertPool
}

// parseURLWithOptionalScheme parses an input string
// that may exclude a scheme, i.e. missing "http://".
// If no scheme is provided, the default scheme
// for the apiserver will be used.
func parseURLWithOptionalScheme(addr string) (*url.URL, bool) {
	// Add the schema if parsing fails or the host is empty.
	// This avoids parsing ambiguity in url.Parse.
	// We default to secure websockets (wss).
	url, err := url.Parse(addr)
	if err != nil || url.Host == "" {
		url, err = url.Parse(apiScheme + "://" + addr)
		if err != nil {
			return nil, false
		}
	}
	return url, true
}

// dialAPI establishes a websocket connection to the RPC
// API websocket on the API server using Info. If multiple API addresses
// are provided in Info they will be tried concurrently - the first successful
// connection wins.
//
// It also returns the TLS configuration that it has derived from the Info.
func dialAPI(ctx context.Context, info *Info, opts0 DialOpts) (*dialResult, error) {
	if len(info.Addrs) == 0 {
		return nil, errors.New("no API addresses to connect to")
	}

	addrs := make([]*url.URL, 0, len(info.Addrs))
	for _, addr := range info.Addrs {
		url, ok := parseURLWithOptionalScheme(addr)
		if !ok {
			logger.Debugf("%q is not a valid URL", addr)
			continue
		}
		// NB: Here we can enforce that the URL scheme is wss
		// or we can allow the user to specify the scheme.
		// Currently it is possible for the user to leave the scheme
		// off and we will default to wss or specify ws (insecure).
		addrs = append(addrs, url)
	}

	if info.Proxier != nil {
		if err := info.Proxier.Start(); err != nil {
			return nil, errors.Annotate(err, "starting proxy for api connection")
		}
		logger.Debugf("starting proxier for connection")

		switch p := info.Proxier.(type) {
		case jujuproxy.TunnelProxier:
			logger.Debugf("tunnel proxy in use at %s on port %s", p.Host(), p.Port())
			addrs = []*url.URL{{
				Scheme: apiScheme,
				Host:   net.JoinHostPort(p.Host(), p.Port()),
			}}
		default:
			info.Proxier.Stop()
			return nil, errors.New("unknown proxier provided")
		}
	}

	opts := dialOpts{
		DialOpts:    opts0,
		sniHostName: info.SNIHostName,
	}
	if info.CACert != "" {
		certPool, err := CreateCertPool(info.CACert)
		if err != nil {
			return nil, errors.Annotate(err, "cert pool creation failed")
		}
		opts.certPool = certPool
	}
	// Set opts.DialWebsocket and opts.Clock here rather than in open because
	// some tests call dialAPI directly.
	if opts.DialWebsocket == nil {
		opts.DialWebsocket = gorillaDialWebsocket
	}
	if opts.IPAddrResolver == nil {
		opts.IPAddrResolver = net.DefaultResolver
	}
	if opts.Clock == nil {
		opts.Clock = clock.WallClock
	}
	if opts.DNSCache == nil {
		opts.DNSCache = nopDNSCache{}
	}

	path, err := apiPath(info.ModelTag.Id(), "/api")
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Encourage load balancing by shuffling controller addresses.
	rand.Shuffle(len(addrs), func(i, j int) { addrs[i], addrs[j] = addrs[j], addrs[i] })

	if opts.VerifyCA != nil {
		if err := verifyCAMulti(ctx, addrs, &opts); err != nil {
			return nil, err
		}
	}

	if opts.DialTimeout > 0 {
		ctx1, cancel := utils.ContextWithTimeout(ctx, opts.Clock, opts.DialTimeout)
		defer cancel()
		ctx = ctx1
	}
	dialInfo, err := dialWebsocketMulti(ctx, addrs, path, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("connection established to %q", dialInfo.dialAddr.String())
	dialInfo.proxier = info.Proxier
	return dialInfo, nil
}

// gorillaDialWebsocket makes a websocket connection using the
// gorilla websocket package. The ipAddr parameter holds the
// actual IP address that will be contacted - the host in urlStr
// is used only for TLS verification when tlsConfig.ServerName
// is empty.
func gorillaDialWebsocket(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(rogpeppe) We'd like to set Deadline here
	// but that would break lots of tests that rely on
	// setting a zero timeout.
	netDialer := net.Dialer{}
	dialer := &websocket.Dialer{
		NetDial: func(netw, addr string) (net.Conn, error) {
			if addr == url.Host {
				// Use pre-resolved IP address. The address
				// may be different if a proxy is in use.
				addr = ipAddr
			}
			return netDialer.DialContext(ctx, netw, addr)
		},
		Proxy:            proxy.DefaultConfig.GetProxy,
		HandshakeTimeout: 45 * time.Second,
		TLSClientConfig:  tlsConfig,
	}
	// Note: no extra headers.
	c, resp, err := dialer.Dial(urlStr, nil)
	if err != nil {
		if err == websocket.ErrBadHandshake {
			// If ErrBadHandshake is returned, a non-nil response
			// is returned so the client can react to auth errors
			// (for example).
			defer resp.Body.Close()
			body, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				err = errors.Errorf(
					"%s (%s)",
					strings.TrimSpace(string(body)),
					http.StatusText(resp.StatusCode),
				)
			}
		}
		return nil, errors.Trace(err)
	}
	return jsoncodec.NewWebsocketConn(c), nil
}

type resolvedAddress struct {
	ip   string
	port string
	url  *url.URL
}

// addressProvider provides a mechanism for resolving
// host addresses to IP addresses. It utilises a DNS cache and an IP
// address resolver to manage and resolve addresses. It maintains
// separate pools for addresses that need to be resolved and those that
// have been resolved via the DNS cache.
type addressProvider struct {
	dnsCache       DNSCache
	ipAddrResolver IPAddrResolver

	// A pool of host URLs to be resolved to one or more IP addresses.
	addrPool []*url.URL

	// A pool of host addresses that got resolved via the DNS cache; these
	// are kept separate so we can attempt to resolve them without the DNS
	// cache when we run out of entries in AddrPool.
	cachedAddrPool []*url.URL
	resolvedAddrs  []*resolvedAddress
}

// newAddressProvider returns a new addressProvider.
func newAddressProvider(initialAddrs []*url.URL, dnsCache DNSCache, ipAddrResolver IPAddrResolver) *addressProvider {
	return &addressProvider{
		dnsCache:       dnsCache,
		ipAddrResolver: ipAddrResolver,
		addrPool:       initialAddrs,
	}
}

// next returns back either a successfully resolved address or the error that
// occurred while attempting to resolve the next address candidate. Calls to
// next return io.EOF to indicate that no more addresses are available.
func (ap *addressProvider) next(ctx context.Context) (*resolvedAddress, error) {
	if len(ap.resolvedAddrs) == 0 {
		// If we have ran out of addresses to resolve but we have
		// resolved some via the DNS cache, make another pass for
		// those with an empty DNS cache to refresh any stale entries.
		if len(ap.addrPool) == 0 && len(ap.cachedAddrPool) > 0 {
			ap.addrPool = ap.cachedAddrPool
			ap.cachedAddrPool = nil
			ap.dnsCache = emptyDNSCache{ap.dnsCache}
		}

		// Resolve the next host from the address pool
		if len(ap.addrPool) != 0 {
			next := ap.addrPool[0]
			ap.addrPool = ap.addrPool[1:]

			host := next.Hostname()

			ips := ap.dnsCache.Lookup(host)
			if len(ips) > 0 {
				ap.cachedAddrPool = append(ap.cachedAddrPool, next)
			} else if isNumericHost(host) {
				ips = []string{host}
			} else {
				var err error
				ips, err = lookupIPAddr(ctx, host, ap.ipAddrResolver)
				if err != nil {
					return nil, errors.Errorf("cannot resolve %q: %v", host, err)
				}
				ap.dnsCache.Add(host, ips)
				logger.Debugf("looked up %v -> %v", host, ips)
			}

			for _, ip := range ips {
				ap.resolvedAddrs = append(ap.resolvedAddrs, &resolvedAddress{
					ip:   ip,
					port: next.Port(),
					url:  next,
				})
			}
		}
	}

	// Ran out of resolved addresses and cached addresses
	if len(ap.resolvedAddrs) == 0 {
		return nil, io.EOF
	}

	next := ap.resolvedAddrs[0]
	ap.resolvedAddrs = ap.resolvedAddrs[1:]
	return next, nil
}

// caRetrieveRes is an adaptor for returning CA certificate lookup results via
// calls to parallel.Try.
type caRetrieveRes struct {
	host     string
	endpoint string
	caCert   *x509.Certificate
}

func (caRetrieveRes) Close() error { return nil }

// verifyCAMulti attempts to establish a TLS connection with one of the
// provided addresses, retrieve the CA certificate and validate it using the
// system root CAs. If that is not possible, the certificate verification will
// be delegated to the VerifyCA implementation specified in opts.DialOpts.
//
// If VerifyCA does not return an error, the CA cert is assumed to be trusted
// and will be appended to opt's certificate pool allowing secure websocket
// connections to proceed without certificate verification errors. Otherwise,
// the error reported by VerifyCA is returned back to the caller.
//
// For load-balancing purposes, all addresses are tested concurrently with the
// first retrieved CA cert being used for the verification tests. In addition,
// apart from the initial TLS handshake with the remote server, no other data
// is exchanged with the remote server.
func verifyCAMulti(ctx context.Context, addrs []*url.URL, opts *dialOpts) error {
	dOpts := opts.DialOpts
	if dOpts.DialTimeout > 0 {
		ctx1, cancel := utils.ContextWithTimeout(ctx, dOpts.Clock, dOpts.DialTimeout)
		defer cancel()
		ctx = ctx1
	}

	try := parallel.NewTry(0, nil)
	defer try.Kill()

	addrProvider := newAddressProvider(addrs, opts.DNSCache, opts.IPAddrResolver)
	tryRetrieveCaCertFn := func(ctx context.Context, addr *resolvedAddress) func(<-chan struct{}) (io.Closer, error) {
		ipStr := net.JoinHostPort(addr.ip, addr.port)
		return func(<-chan struct{}) (io.Closer, error) {
			caCert, err := retrieveCACert(ctx, ipStr)
			if err != nil {
				return nil, err
			}

			return caRetrieveRes{
				host:     addr.url.Host,
				endpoint: addr.ip,
				caCert:   caCert,
			}, nil
		}
	}

	for {
		resolvedAddr, err := addrProvider.next(ctx)
		if err == io.EOF {
			break
		} else if err != nil {
			recordTryError(try, err)
			continue
		}

		err = try.Start(tryRetrieveCaCertFn(ctx, resolvedAddr))
		if err == parallel.ErrStopped {
			break
		} else if err != nil {
			continue
		}

		select {
		case <-opts.Clock.After(dOpts.DialAddressInterval):
		case <-try.Dead():
		}
	}

	try.Close()

	// If we are unable to fetch the CA either because it is not presented
	// by the remote server OR due to an unsuccessful connection attempt
	// we should skip the verification path and dial the server as if no
	// VerifyCA implementation was provided.
	result, err := try.Result()
	if err != nil || result == nil {
		logger.Debugf("unable to retrieve CA cert from remote host; skipping CA verification")
		return nil
	}

	// Try to verify CA cert using the system roots. If the verification
	// succeeds then we are done; tls connections will work out of the box.
	res := result.(caRetrieveRes)
	if _, err = res.caCert.Verify(x509.VerifyOptions{}); err == nil {
		logger.Debugf("remote CA certificate trusted by system roots")
		return nil
	}

	// Invoke the CA verifier; if the CA should be trusted, append it to
	// the dialOpts certPool and proceed with the actual connection attempt.
	err = opts.VerifyCA(res.host, res.endpoint, res.caCert)
	if err == nil {
		if opts.certPool == nil {
			opts.certPool = x509.NewCertPool()
		}
		opts.certPool.AddCert(res.caCert)
	}

	return err
}

// retrieveCACert establishes an insecure TLS connection to addr and attempts
// to retrieve the CA cert presented by the server. If no CA cert is presented,
// retrieveCACert will returns nil, nil.
func retrieveCACert(ctx context.Context, addr string) (*x509.Certificate, error) {
	netConn, err := new(net.Dialer).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	conn := tls.Client(netConn, &tls.Config{InsecureSkipVerify: true})
	if err = conn.Handshake(); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	defer func() {
		_ = conn.Close()
		_ = netConn.Close()
	}()

	for _, cert := range conn.ConnectionState().PeerCertificates {
		if cert.IsCA {
			return cert, nil
		}
	}

	return nil, errors.New("no CA certificate presented by remote server")
}

// dialWebsocketMulti dials a websocket with one of the provided addresses, the
// specified URL path, TLS configuration, and dial options. Each of the
// specified addresses will be attempted concurrently, and the first
// successful connection will be returned.
// The apiPath is appended to the URL of each address.
func dialWebsocketMulti(ctx context.Context, addrs []*url.URL, apiPath string, opts dialOpts) (*dialResult, error) {
	// Prioritise non-dial errors over the normal "connection refused".
	isDialError := func(err error) bool {
		netErr, ok := errors.Cause(err).(*net.OpError)
		if !ok {
			return false
		}
		return netErr.Op == "dial"
	}
	combine := func(initial, other error) error {
		if initial == nil || isDialError(initial) {
			return other
		}
		if isDialError(other) {
			return initial
		}
		return other
	}
	// Dial all addresses at reasonable intervals.
	try := parallel.NewTry(0, combine)
	defer try.Kill()
	// Make a context that's cancelled when the try
	// completes so that (for example) a slow DNS
	// query will be cancelled if a previous try succeeds.
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-try.Dead()
		cancel()
	}()
	tried := make(map[string]bool)
	addrProvider := newAddressProvider(addrs, opts.DNSCache, opts.IPAddrResolver)
	for {
		resolvedAddr, err := addrProvider.next(ctx)
		if err == io.EOF {
			break
		} else if err != nil {
			recordTryError(try, err)
			continue
		}

		ipStr := net.JoinHostPort(resolvedAddr.ip, resolvedAddr.port)
		if tried[ipStr] {
			continue
		}
		tried[ipStr] = true

		// Make a copy of the controller's root URL before we append the API path.
		// The original URL should be preserved because it too may contain a path segment
		// if a controller is hosted on a subpath, e.g. https://controller.com/foo/bar.
		urlWithAPIPath := *resolvedAddr.url
		urlWithAPIPath.Path = gopath.Join(urlWithAPIPath.Path, apiPath)

		err = startDialWebsocket(ctx, try, resolvedAddr.url, &urlWithAPIPath, ipStr, opts)
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

func lookupIPAddr(ctx context.Context, host string, resolver IPAddrResolver) ([]string, error) {
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ips := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr.Zone != "" {
			// Ignore IPv6 zone. Hopefully this shouldn't
			// cause any problems in practice.
			logger.Infof("ignoring IP address with zone %q", addr)
			continue
		}
		ips = append(ips, addr.IP.String())
	}
	return ips, nil
}

// recordTryError starts a try that just returns the given error.
// This is so that we can use the usual Try error combination
// logic even for errors that happen before we start a try.
func recordTryError(try *parallel.Try, err error) {
	logger.Infof("%v", err)
	_ = try.Start(func(_ <-chan struct{}) (io.Closer, error) {
		return nil, errors.Trace(err)
	})
}

var oneAttempt = retry.LimitCount(1, retry.Regular{
	Min: 1,
})

// startDialWebsocket starts websocket connection to a single address
// on the given try instance.
func startDialWebsocket(ctx context.Context, try *parallel.Try, rootAddr, addr *url.URL, ipAddr string, opts dialOpts) error {
	var openAttempt retry.Strategy
	if opts.RetryDelay > 0 {
		openAttempt = retry.Regular{
			Total: opts.Timeout,
			Delay: opts.RetryDelay,
			Min:   int(opts.Timeout / opts.RetryDelay),
		}
	} else {
		// Zero retry delay implies exactly one try.
		openAttempt = oneAttempt
	}
	d := dialer{
		ctx:            ctx,
		openAttempt:    openAttempt,
		serverName:     opts.sniHostName,
		ipAddr:         ipAddr,
		addr:           addr,
		controllerRoot: rootAddr,
		opts:           opts,
	}
	return try.Start(d.dial)
}

type dialer struct {
	ctx         context.Context
	openAttempt retry.Strategy

	// serverName holds the SNI name to use
	// when connecting with a public certificate.
	serverName string

	// addr holds the address that is being dialed,
	// including any API specific paths
	// e.g. wss://controller.com/foo/bar/api.
	addr *url.URL

	// controllerRoot represents the root URL of the controller
	// e.g. wss://controller.com/foo/bar.
	controllerRoot *url.URL

	// addr holds the ipaddr:port (one of the addresses
	// that addr resolves to) that is being dialed.
	ipAddr string

	// opts holds the dial options.
	opts dialOpts
}

// dial implements the function value expected by Try.Start
// by dialing the websocket as specified in d and retrying
// when appropriate.
func (d dialer) dial(_ <-chan struct{}) (io.Closer, error) {
	a := retry.StartWithCancel(d.openAttempt, d.opts.Clock, d.ctx.Done())
	var lastErr error = nil
	for a.Next() {
		conn, tlsConfig, err := d.dial1()
		if err == nil {
			return &dialResult{
				conn:               conn,
				dialAddr:           d.addr,
				controllerRootAddr: d.controllerRoot,
				ipAddr:             d.ipAddr,
				tlsConfig:          tlsConfig,
			}, nil
		}
		if isX509Error(err) || !a.More() {
			// certificate errors don't improve with retries.
			return nil, errors.Annotatef(err, "unable to connect to API")
		}
		lastErr = err
	}
	if lastErr == nil {
		logger.Debugf("no error, but not connected, probably cancelled before we started")
		return nil, parallel.ErrStopped
	}
	return nil, errors.Trace(lastErr)
}

// dial1 makes a single dial attempt.
func (d dialer) dial1() (jsoncodec.JSONConn, *tls.Config, error) {
	tlsConfig := NewTLSConfig(d.opts.certPool)
	tlsConfig.InsecureSkipVerify = d.opts.InsecureSkipVerify
	if d.opts.certPool == nil {
		tlsConfig.ServerName = d.serverName
	}
	logger.Tracef("dialing: %q %v", d.addr.String(), d.ipAddr)
	conn, err := d.opts.DialWebsocket(d.ctx, d.addr.String(), tlsConfig, d.ipAddr)
	if err == nil {
		logger.Debugf("successfully dialed %q", d.addr.String())
		return conn, tlsConfig, nil
	}
	if !isX509Error(err) {
		return nil, nil, errors.Trace(err)
	}
	if tlsConfig.RootCAs == nil || d.serverName == "" {
		// There's no private certificate or we don't have a
		// public hostname. In the former case, we've already
		// tried public certificates; in the latter, public cert
		// validation won't help, because you generally can't
		// obtain a public cert for a numeric IP address. In
		// both those cases, we won't succeed when trying again
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
	tlsConfig.RootCAs = nil
	tlsConfig.ServerName = d.serverName
	conn, rootCAErr := d.opts.DialWebsocket(d.ctx, d.addr.String(), tlsConfig, d.ipAddr)
	if rootCAErr != nil {
		logger.Debugf("failed to dial websocket using fallback public CA: %v", rootCAErr)
		// We return the original error as it's usually more meaningful.
		return nil, nil, errors.Trace(err)
	}
	return conn, tlsConfig, nil
}

// NewTLSConfig returns a new *tls.Config suitable for connecting to a Juju
// API server. If certPool is non-nil, we use it as the config's RootCAs,
// and the server name is set to "juju-apiserver".
func NewTLSConfig(certPool *x509.CertPool) *tls.Config {
	tlsConfig := jujuhttp.SecureTLSConfig()
	if certPool != nil {
		// We want to be specific here (rather than just using "anything").
		// See commit 7fc118f015d8480dfad7831788e4b8c0432205e8 (PR 899).
		tlsConfig.RootCAs = certPool
		tlsConfig.ServerName = "juju-apiserver"
	}
	return tlsConfig
}

// isNumericHost reports whether the given host name is
// a numeric IP address.
func isNumericHost(host string) bool {
	return net.ParseIP(host) != nil
}

// isX509Error reports whether the given websocket error
// results from an X509 problem.
func isX509Error(err error) bool {
	// Check early close of websocket during TLS handshake
	var closeError *websocket.CloseError
	if errors.As(err, &closeError) {
		return closeError.Code == websocket.CloseTLSHandshake
	}

	// Check various tls error
	var (
		certificateInvalidError    x509.CertificateInvalidError
		hostnameError              x509.HostnameError
		insecureAlgorithmError     x509.InsecureAlgorithmError
		unhandledCriticalExtension x509.UnhandledCriticalExtension
		unknownAuthorityError      x509.UnknownAuthorityError
		constraintViolationError   x509.ConstraintViolationError
		systemRootsError           x509.SystemRootsError
	)

	if errors.As(err, &certificateInvalidError) ||
		errors.As(err, &hostnameError) ||
		errors.As(err, &insecureAlgorithmError) ||
		errors.As(err, &unhandledCriticalExtension) ||
		errors.As(err, &unknownAuthorityError) ||
		errors.As(err, &constraintViolationError) ||
		errors.As(err, &systemRootsError) {
		return true
	}
	return false
}

// APICall places a call to the remote machine.
//
// This fills out the rpc.Request on the given facade, version for a given
// object id, and the specific RPC method. It marshalls the Arguments, and will
// unmarshall the result into the response object that is supplied.
func (s *state) APICall(facade string, vers int, id, method string, args, response interface{}) error {
	err := s.client.Call(rpc.Request{
		Type:    facade,
		Version: vers,
		Id:      id,
		Action:  method,
	}, args, response)

	if code := params.ErrCode(err); code == params.CodeNotImplemented {
		return errors.NewNotImplemented(fmt.Errorf("%w\nre-install your juju client to match the version running on the controller", err), "\njuju client not compatible with server")
	}
	return errors.Trace(err)
}

func (s *state) Close() error {
	// The bakery client for Macaroons uses a round-tripper, so it is not a
	// connection in its own right, but we can get it to close any connections
	// from completed requests that are still open.
	// Note that this does nothing for connections in use, including those
	// held open because someone failed to close a HTTP response body.
	s.bakeryClient.Client.CloseIdleConnections()

	err := s.client.Close()
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	<-s.broken
	if s.proxier != nil {
		s.proxier.Stop()
	}
	return err
}

// wrappedBakeryClient wraps an httpbakery.Client
// to implement base.MacaroonDischarger.
type wrappedBakeryClient struct {
	*httpbakery.Client
}

// CookieJar implements base.MacaroonDischarger.
// It returns the cookie jar used by the bakery client.
func (w *wrappedBakeryClient) CookieJar() http.CookieJar {
	return w.Client.Jar
}

// BakeryClient implements api.Connection.
func (s *state) BakeryClient() base.MacaroonDischarger {
	return &wrappedBakeryClient{s.bakeryClient}
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
	if err := s.ping(); err != nil {
		logger.Debugf("connection ping failed: %v", err)
		return true
	}
	return false
}

// Addr returns a copy of the url used to connect to the API server.
func (s *state) Addr() *url.URL {
	copy := *s.addr
	return &copy
}

// IPAddr returns the resolved IP address that was used to
// connect to the API server.
func (s *state) IPAddr() string {
	return s.ipAddr
}

// IsProxied indicates if this connection was proxied
func (s *state) IsProxied() bool {
	return s.proxier != nil
}

// Proxy returns the proxy being used with this connection if one is being used.
func (s *state) Proxy() jujuproxy.Proxier {
	return s.proxier
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
// to the API server. It includes the address used to connect
// when the address does not include a path segment.
//
// The addresses are scoped (public, cloud-internal, etc.), so
// the client may choose which addresses to attempt. For the
// Juju CLI, all addresses must be attempted, as the CLI may
// be invoked both within and outside the model (think
// private clouds).
func (s *state) APIHostPorts() []network.MachineHostPorts {
	// NOTE: We're making a copy of s.hostPorts before returning it,
	// for safety.
	hostPorts := make([]network.MachineHostPorts, len(s.hostPorts))
	for i, servers := range s.hostPorts {
		hostPorts[i] = append(network.MachineHostPorts{}, servers...)
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

// BestFacadeVersion compares the versions of facades that we know about, and
// the versions available from the server, and reports back what version is the
// 'best available' to use.
// TODO(jam) this is the eventual implementation of what version of a given
// Facade we will want to use. It needs to line up the versions that the server
// reports to us, with the versions that our client knows how to use.
func (s *state) BestFacadeVersion(facade string) int {
	return facades.BestVersion(facadeVersions[facade], s.facadeVersions[facade])
}

func (s *state) isLoggedIn() bool {
	return atomic.LoadInt32(&s.loggedIn) == 1
}

func (s *state) setLoggedIn() {
	atomic.StoreInt32(&s.loggedIn, 1)
}

// emptyDNSCache implements DNSCache by
// never returning any entries but writing any
// added entries to the embedded DNSCache object.
type emptyDNSCache struct {
	DNSCache
}

func (emptyDNSCache) Lookup(host string) []string {
	return nil
}

type nopDNSCache struct{}

func (nopDNSCache) Lookup(host string) []string {
	return nil
}

func (nopDNSCache) Add(host string, ips []string) {
}
