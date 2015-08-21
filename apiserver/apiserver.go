// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bmizerany/pat"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver")

// loginRateLimit defines how many concurrent Login requests we will
// accept
const loginRateLimit = 10

// Server holds the server side of the API.
type Server struct {
	tomb              tomb.Tomb
	wg                sync.WaitGroup
	state             *state.State
	statePool         *state.StatePool
	addr              *net.TCPAddr
	tag               names.Tag
	dataDir           string
	logDir            string
	limiter           utils.Limiter
	validator         LoginValidator
	adminApiFactories map[int]adminApiFactory

	mu          sync.Mutex // protects the fields that follow
	environUUID string
}

// LoginValidator functions are used to decide whether login requests
// are to be allowed. The validator is called before credentials are
// checked.
type LoginValidator func(params.LoginRequest) error

// ServerConfig holds parameters required to set up an API server.
type ServerConfig struct {
	Cert        []byte
	Key         []byte
	Tag         names.Tag
	DataDir     string
	LogDir      string
	Validator   LoginValidator
	CertChanged chan params.StateServingInfo
}

// changeCertListener wraps a TLS net.Listener.
// It allows connection handshakes to be
// blocked while the TLS certificate is updated.
type changeCertListener struct {
	net.Listener
	tomb tomb.Tomb

	// A mutex used to block accept operations.
	m sync.Mutex

	// A channel used to pass in new certificate information.
	certChanged <-chan params.StateServingInfo

	// The config to update with any new certificate.
	config tls.Config
}

func newChangeCertListener(lis net.Listener, certChanged <-chan params.StateServingInfo, config tls.Config) *changeCertListener {
	cl := &changeCertListener{
		Listener:    lis,
		certChanged: certChanged,
		config:      config,
	}
	go func() {
		defer cl.tomb.Done()
		cl.tomb.Kill(cl.processCertChanges())
	}()
	return cl
}

// Accept waits for and returns the next connection to the listener.
func (cl *changeCertListener) Accept() (net.Conn, error) {
	conn, err := cl.Listener.Accept()
	if err != nil {
		return nil, err
	}
	cl.m.Lock()
	defer cl.m.Unlock()
	config := cl.config
	return tls.Server(conn, &config), nil
}

// Close closes the listener.
func (cl *changeCertListener) Close() error {
	cl.tomb.Kill(nil)
	return cl.Listener.Close()
}

// processCertChanges receives new certificate information and
// calls a method to update the listener's certificate.
func (cl *changeCertListener) processCertChanges() error {
	for {
		select {
		case info := <-cl.certChanged:
			if info.Cert != "" {
				cl.updateCertificate([]byte(info.Cert), []byte(info.PrivateKey))
			}
		case <-cl.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// updateCertificate generates a new TLS certificate and assigns it
// to the TLS listener.
func (cl *changeCertListener) updateCertificate(cert, key []byte) {
	cl.m.Lock()
	defer cl.m.Unlock()
	if tlsCert, err := tls.X509KeyPair(cert, key); err != nil {
		logger.Errorf("cannot create new TLS certificate: %v", err)
	} else {
		logger.Infof("updating api server certificate")
		x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err == nil {
			var addr []string
			for _, ip := range x509Cert.IPAddresses {
				addr = append(addr, ip.String())
			}
			logger.Infof("new certificate addresses: %v", strings.Join(addr, ", "))
		}
		cl.config.Certificates = []tls.Certificate{tlsCert}
	}
}

// NewServer serves the given state by accepting requests on the given
// listener, using the given certificate and key (in PEM format) for
// authentication.
func NewServer(s *state.State, lis net.Listener, cfg ServerConfig) (*Server, error) {
	l, ok := lis.(*net.TCPListener)
	if !ok {
		return nil, errors.Errorf("listener is not of type *net.TCPListener: %T", lis)
	}
	return newServer(s, l, cfg)
}

func newServer(s *state.State, lis *net.TCPListener, cfg ServerConfig) (*Server, error) {
	logger.Infof("listening on %q", lis.Addr())
	srv := &Server{
		state:     s,
		statePool: state.NewStatePool(s),
		addr:      lis.Addr().(*net.TCPAddr), // cannot fail
		tag:       cfg.Tag,
		dataDir:   cfg.DataDir,
		logDir:    cfg.LogDir,
		limiter:   utils.NewLimiter(loginRateLimit),
		validator: cfg.Validator,
		adminApiFactories: map[int]adminApiFactory{
			0: newAdminApiV0,
			1: newAdminApiV1,
			2: newAdminApiV2,
		},
	}
	tlsCert, err := tls.X509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, err
	}
	// TODO(rog) check that *srvRoot is a valid type for using
	// as an RPC server.
	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
	changeCertListener := newChangeCertListener(lis, cfg.CertChanged, tlsConfig)
	go srv.run(changeCertListener)
	return srv, nil
}

// Dead returns a channel that signals when the server has exited.
func (srv *Server) Dead() <-chan struct{} {
	return srv.tomb.Dead()
}

// Stop stops the server and returns when all running requests
// have completed.
func (srv *Server) Stop() error {
	srv.tomb.Kill(nil)
	return srv.tomb.Wait()
}

// Kill implements worker.Worker.Kill.
func (srv *Server) Kill() {
	srv.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (srv *Server) Wait() error {
	return srv.tomb.Wait()
}

type requestNotifier struct {
	id    int64
	start time.Time

	mu   sync.Mutex
	tag_ string
}

var globalCounter int64

func newRequestNotifier() *requestNotifier {
	return &requestNotifier{
		id:    atomic.AddInt64(&globalCounter, 1),
		tag_:  "<unknown>",
		start: time.Now(),
	}
}

func (n *requestNotifier) login(tag string) {
	n.mu.Lock()
	n.tag_ = tag
	n.mu.Unlock()
}

func (n *requestNotifier) tag() (tag string) {
	n.mu.Lock()
	tag = n.tag_
	n.mu.Unlock()
	return
}

func (n *requestNotifier) ServerRequest(hdr *rpc.Header, body interface{}) {
	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some requests.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if logger.IsTraceEnabled() {
		logger.Tracef("<- [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, body))
	} else {
		logger.Debugf("<- [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, "'params redacted'"))
	}
}

func (n *requestNotifier) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}, timeSpent time.Duration) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some responses.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if logger.IsTraceEnabled() {
		logger.Tracef("-> [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, body))
	} else {
		logger.Debugf("-> [%X] %s %s %s %s[%q].%s", n.id, n.tag(), timeSpent, jsoncodec.DumpRequest(hdr, "'body redacted'"), req.Type, req.Id, req.Action)
	}
}

func (n *requestNotifier) join(req *http.Request) {
	logger.Infof("[%X] API connection from %s", n.id, req.RemoteAddr)
}

func (n *requestNotifier) leave() {
	logger.Infof("[%X] %s API connection terminated after %v", n.id, n.tag(), time.Since(n.start))
}

func (n *requestNotifier) ClientRequest(hdr *rpc.Header, body interface{}) {
}

func (n *requestNotifier) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
}

func handleAll(mux *pat.PatternServeMux, pattern string, handler http.Handler) {
	mux.Get(pattern, handler)
	mux.Post(pattern, handler)
	mux.Head(pattern, handler)
	mux.Put(pattern, handler)
	mux.Del(pattern, handler)
	mux.Options(pattern, handler)
}

func (srv *Server) run(lis net.Listener) {
	defer func() {
		srv.state.HackLeadership() // Break deadlocks caused by BlockUntil... calls.
		srv.wg.Wait()              // wait for any outstanding requests to complete.
		srv.tomb.Done()
		srv.statePool.Close()
	}()

	srv.wg.Add(1)
	go func() {
		err := srv.mongoPinger()
		srv.tomb.Kill(err)
		srv.wg.Done()
	}()

	// for pat based handlers, they are matched in-order of being
	// registered, first match wins. So more specific ones have to be
	// registered first.
	mux := pat.New()

	srvDying := srv.tomb.Dying()

	if feature.IsDbLogEnabled() {
		handleAll(mux, "/environment/:envuuid/logsink",
			newLogSinkHandler(httpHandler{statePool: srv.statePool}, srv.logDir))
		handleAll(mux, "/environment/:envuuid/log",
			newDebugLogDBHandler(srv.statePool, srvDying))
	} else {
		handleAll(mux, "/environment/:envuuid/log",
			newDebugLogFileHandler(srv.statePool, srvDying, srv.logDir))
	}
	handleAll(mux, "/environment/:envuuid/charms",
		&charmsHandler{
			httpHandler: httpHandler{statePool: srv.statePool},
			dataDir:     srv.dataDir},
	)
	// TODO: We can switch from handleAll to mux.Post/Get/etc for entries
	// where we only want to support specific request methods. However, our
	// tests currently assert that errors come back as application/json and
	// pat only does "text/plain" responses.
	handleAll(mux, "/environment/:envuuid/tools",
		&toolsUploadHandler{toolsHandler{
			httpHandler{statePool: srv.statePool},
		}},
	)
	handleAll(mux, "/environment/:envuuid/tools/:version",
		&toolsDownloadHandler{toolsHandler{
			httpHandler{statePool: srv.statePool},
		}},
	)
	handleAll(mux, "/environment/:envuuid/backups",
		&backupHandler{httpHandler{
			statePool:          srv.statePool,
			strictValidation:   true,
			stateServerEnvOnly: true,
		}},
	)
	handleAll(mux, "/environment/:envuuid/api", http.HandlerFunc(srv.apiHandler))
	handleAll(mux, "/environment/:envuuid/images/:kind/:series/:arch/:filename",
		&imagesDownloadHandler{
			httpHandler: httpHandler{statePool: srv.statePool},
			dataDir:     srv.dataDir},
	)
	// For backwards compatibility we register all the old paths

	if feature.IsDbLogEnabled() {
		handleAll(mux, "/log", newDebugLogDBHandler(srv.statePool, srvDying))
	} else {
		handleAll(mux, "/log", newDebugLogFileHandler(srv.statePool, srvDying, srv.logDir))
	}

	handleAll(mux, "/charms",
		&charmsHandler{
			httpHandler: httpHandler{statePool: srv.statePool},
			dataDir:     srv.dataDir,
		},
	)
	handleAll(mux, "/tools",
		&toolsUploadHandler{toolsHandler{
			httpHandler{statePool: srv.statePool},
		}},
	)
	handleAll(mux, "/tools/:version",
		&toolsDownloadHandler{toolsHandler{
			httpHandler{statePool: srv.statePool},
		}},
	)
	handleAll(mux, "/", http.HandlerFunc(srv.apiHandler))

	go func() {
		// The error from http.Serve is not interesting.
		http.Serve(lis, mux)
	}()

	<-srv.tomb.Dying()
	lis.Close()
}

func (srv *Server) apiHandler(w http.ResponseWriter, req *http.Request) {
	reqNotifier := newRequestNotifier()
	reqNotifier.join(req)
	defer reqNotifier.leave()
	wsServer := websocket.Server{
		Handler: func(conn *websocket.Conn) {
			srv.wg.Add(1)
			defer srv.wg.Done()
			// If we've got to this stage and the tomb is still
			// alive, we know that any tomb.Kill must occur after we
			// have called wg.Add, so we avoid the possibility of a
			// handler goroutine running after Stop has returned.
			if srv.tomb.Err() != tomb.ErrStillAlive {
				return
			}
			envUUID := req.URL.Query().Get(":envuuid")
			logger.Tracef("got a request for env %q", envUUID)
			if err := srv.serveConn(conn, reqNotifier, envUUID); err != nil {
				logger.Errorf("error serving RPCs: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() *net.TCPAddr {
	return srv.addr
}

func (srv *Server) serveConn(wsConn *websocket.Conn, reqNotifier *requestNotifier, envUUID string) error {
	codec := jsoncodec.NewWebsocket(wsConn)
	if loggo.GetLogger("juju.rpc.jsoncodec").EffectiveLogLevel() <= loggo.TRACE {
		codec.SetLogging(true)
	}
	var notifier rpc.RequestNotifier
	if logger.EffectiveLogLevel() <= loggo.DEBUG {
		// Incur request monitoring overhead only if we
		// know we'll need it.
		notifier = reqNotifier
	}
	conn := rpc.NewConn(codec, notifier)

	var h *apiHandler
	st, err := validateEnvironUUID(validateArgs{statePool: srv.statePool, envUUID: envUUID})
	if err == nil {
		h, err = newApiHandler(srv, st, conn, reqNotifier, envUUID)
	}
	if err != nil {
		conn.Serve(&errRoot{err}, serverError)
	} else {
		adminApis := make(map[int]interface{})
		for apiVersion, factory := range srv.adminApiFactories {
			adminApis[apiVersion] = factory(srv, h, reqNotifier)
		}
		conn.ServeFinder(newAnonRoot(h, adminApis), serverError)
	}
	conn.Start()
	select {
	case <-conn.Dead():
	case <-srv.tomb.Dying():
	}
	return conn.Close()
}

func (srv *Server) mongoPinger() error {
	timer := time.NewTimer(0)
	session := srv.state.MongoSession()
	for {
		select {
		case <-timer.C:
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		}
		if err := session.Ping(); err != nil {
			logger.Infof("got error pinging mongo: %v", err)
			return errors.Annotate(err, "error pinging mongo")
		}
		timer.Reset(mongoPingInterval)
	}
}

func serverError(err error) error {
	if err := common.ServerError(err); err != nil {
		return err
	}
	return nil
}

var logRequests = true
