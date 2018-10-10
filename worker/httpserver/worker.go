// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pubsub/apiserver"
)

var logger = loggo.GetLogger("juju.worker.httpserver")

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentConfig          agent.Config
	Clock                clock.Clock
	TLSConfig            *tls.Config
	AutocertHandler      http.Handler
	AutocertListener     net.Listener
	Mux                  *apiserverhttp.Mux
	PrometheusRegisterer prometheus.Registerer
	Hub                  *pubsub.StructuredHub
	APIPort              int
	APIPortOpenDelay     time.Duration
	ControllerAPIPort    int
}

// Validate validates the API server configuration.
func (config Config) Validate() error {
	if config.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	if config.TLSConfig == nil {
		return errors.NotValidf("nil TLSConfig")
	}
	if config.Mux == nil {
		return errors.NotValidf("nil Mux")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.AutocertHandler != nil && config.AutocertListener == nil {
		return errors.NewNotValid(nil, "AutocertListener must not be nil if AutocertHandler is not nil")
	}
	return nil
}

// NewWorker returns a new API server worker, with the given configuration.
func NewWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
		url:    make(chan string),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	url      chan string

	unsub func()
}

func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// URL returns the base URL of the HTTP server of the form
// https://ipaddr:port with no trailing slash.
func (w *Worker) URL() string {
	select {
	case <-w.catacomb.Dying():
		return ""
	case url := <-w.url:
		return url
	}
}

func (w *Worker) loop() error {
	var err error
	var listener listener
	if w.config.ControllerAPIPort == 0 {
		listener, err = w.newSimpleListener()
	} else {
		listener, err = w.newDualPortListener()
	}
	if err != nil {
		return errors.Trace(err)
	}
	holdable := newHeldListener(listener, w.config.Clock)

	serverLog := log.New(&loggoWrapper{
		level:  loggo.WARNING,
		logger: logger,
	}, "", 0) // no prefix and no flags so log.Logger doesn't add extra prefixes
	server := &http.Server{
		Handler:   w.config.Mux,
		TLSConfig: w.config.TLSConfig,
		ErrorLog:  serverLog,
	}
	go server.Serve(tls.NewListener(holdable, w.config.TLSConfig))
	defer func() {
		logger.Infof("shutting down HTTP server")
		// Shutting down the server will also close listener.
		err := server.Shutdown(context.Background())
		// Release the holdable listener to unblock any pending accepts.
		holdable.release()
		w.catacomb.Kill(err)
	}()

	if w.config.AutocertHandler != nil {
		autocertServer := &http.Server{
			Handler:  w.config.AutocertHandler,
			ErrorLog: serverLog,
		}
		go autocertServer.Serve(w.config.AutocertListener)
		defer func() {
			logger.Infof("shutting down autocert HTTP server")
			// This will also close the autocert listener.
			err := autocertServer.Shutdown(context.Background())
			w.catacomb.Kill(err)
		}()
	}

	for {
		select {
		case <-w.catacomb.Dying():
			// Stop accepting new connections. This allows the mux
			// to process all pending requests without having to deal with
			// new ones.
			holdable.hold()
			// Asked to shutdown - make sure we wait until all clients
			// have finished up.
			w.config.Mux.Wait()
			return w.catacomb.ErrDying()
		case w.url <- listener.URL():
		}
	}
}

type heldListener struct {
	net.Listener
	clock clock.Clock
	mu    sync.Mutex
	held  bool
}

func newHeldListener(l net.Listener, c clock.Clock) *heldListener {
	return &heldListener{
		Listener: l,
		clock:    c,
	}
}

func (h *heldListener) hold() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.held = true
}

func (h *heldListener) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.held = false
}

func (h *heldListener) Accept() (net.Conn, error) {
	// Normally we'd use a channel to signal between goroutines,
	// but in this case where we want accept to be fast in almost all cases,
	// but wait when held, we can't have it selecting on a channel outside
	// of the mutex lock without a race condition. So the safe and slightly
	// icky method is to wait in a for loop.
	h.mu.Lock()
	for h.held {
		h.mu.Unlock()
		<-h.clock.After(100 * time.Millisecond)
		h.mu.Lock()
	}
	h.mu.Unlock()
	return h.Listener.Accept()
}

type listener interface {
	net.Listener
	URL() string
}

func (w *Worker) newSimpleListener() (listener, error) {
	listenAddr := net.JoinHostPort("", strconv.Itoa(w.config.APIPort))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("listening on %q", listener.Addr())
	return &simpleListener{listener}, nil
}

type simpleListener struct {
	net.Listener
}

func (s *simpleListener) URL() string {
	return fmt.Sprintf("https://%s", s.Addr())
}

func (w *Worker) newDualPortListener() (listener, error) {
	// Only open the controller port until we have been told that
	// the controller is ready. This is currently done by the event
	// from the peergrouper.
	// TODO (thumper): make the raft worker publish an event when
	// it knows who the raft master is. This means that this controller
	// is part of the consensus set, and when it is, is is OK to accept
	// agent connections. Until that time, accepting an agent connection
	// would be a bit of a waste of time.
	listenAddr := net.JoinHostPort("", strconv.Itoa(w.config.ControllerAPIPort))
	listener, err := net.Listen("tcp", listenAddr)
	logger.Infof("listening for controller connections on %q", listener.Addr())
	dual := &dualListener{
		clock:              w.config.Clock,
		delay:              w.config.APIPortOpenDelay,
		apiPort:            w.config.APIPort,
		controllerListener: listener,
		done:               make(chan struct{}),
		errors:             make(chan error),
		connections:        make(chan net.Conn),
	}
	go dual.accept(listener)

	dual.unsub, err = w.config.Hub.Subscribe(apiserver.DetailsTopic, dual.openAPIPort)
	if err != nil {
		dual.Close()
		return nil, errors.Annotate(err, "unable to subscribe to details topic")
	}

	return dual, err
}

type dualListener struct {
	clock   clock.Clock
	delay   time.Duration
	apiPort int

	controllerListener net.Listener
	apiListener        net.Listener

	mu     sync.Mutex
	closer sync.Once

	done        chan struct{}
	errors      chan error
	connections chan net.Conn

	unsub func()
}

func (d *dualListener) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case d.errors <- err:
			case <-d.done:
				logger.Infof("no longer accepting connections on %s", listener.Addr())
				return
			}
		} else {
			select {
			case d.connections <- conn:
			case <-d.done:
				conn.Close()
				logger.Infof("no longer accepting connections on %s", listener.Addr())
				return
			}
		}
	}
}

// Accept implements net.Listener.
func (d *dualListener) Accept() (net.Conn, error) {
	select {
	case <-d.done:
		return nil, errors.New("listener has been closed")
	case err := <-d.errors:
		return nil, errors.Trace(err)
	case conn := <-d.connections:
		return conn, nil
	}
}

// Close implements net.Listener. Closes all the open listeners.
func (d *dualListener) Close() error {
	// Only close the channel once.
	d.closer.Do(func() { close(d.done) })
	err := d.controllerListener.Close()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.apiListener != nil {
		err2 := d.apiListener.Close()
		if err == nil {
			err = err2
		}
		// If we already have a close error, we don't really care
		// about this one.
	}
	return errors.Trace(err)
}

// Addr implements net.Listener. If the api port has been opened, we
// return that, otherwise we return the controller port address.
func (d *dualListener) Addr() net.Addr {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.apiListener != nil {
		return d.apiListener.Addr()
	}
	return d.controllerListener.Addr()
}

// URL implements the listener method.
func (d *dualListener) URL() string {
	return fmt.Sprintf("https://%s", d.Addr())
}

// openAPIPort opens the api port and starts accepting connections.
func (d *dualListener) openAPIPort(_ string, _ map[string]interface{}) {
	logger.Infof("waiting for %s before allowing api connections", d.delay)
	<-d.clock.After(d.delay)

	defer d.unsub()

	d.mu.Lock()
	defer d.mu.Unlock()
	// Make sure we haven't been closed already.
	select {
	case <-d.done:
		return
	default:
		// We are all good.
	}

	listenAddr := net.JoinHostPort("", strconv.Itoa(d.apiPort))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		select {
		case d.errors <- err:
		case <-d.done:
			logger.Errorf("can't open api port: %v, but worker exiting already", err)
		}
		return
	}

	logger.Infof("listening for api connections on %q", listener.Addr())
	go d.accept(listener)
}
