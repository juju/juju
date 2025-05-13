// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v4/catacomb"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/pubsub/apiserver"
)

var (
	// ShutdownTimeout is how long the http server will wait for active connections
	// to close.
	ShutdownTimeout = 30 * time.Second
)

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentName            string
	Clock                clock.Clock
	TLSConfig            *tls.Config
	Mux                  *apiserverhttp.Mux
	MuxShutdownWait      time.Duration
	LogDir               string
	Logger               logger.Logger
	PrometheusRegisterer prometheus.Registerer
	Hub                  *pubsub.StructuredHub
	APIPort              int
	APIPortOpenDelay     time.Duration
	ControllerAPIPort    int
}

// Validate validates the API server configuration.
func (config Config) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.TLSConfig == nil {
		return errors.NotValidf("nil TLSConfig")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Mux == nil {
		return errors.NotValidf("nil Mux")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
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
		logger: config.Logger,
		url:    make(chan string),
		status: "starting",
	}
	var err error
	var listener listener
	if w.config.ControllerAPIPort == 0 {
		listener, err = w.newSimpleListener()
	} else {
		listener, err = w.newDualPortListener()
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.holdable = newHeldListener(listener, config.Clock)

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "httpserver",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		listener.Close()
		return nil, errors.Trace(err)
	}
	return w, nil
}

type reporter interface {
	report() map[string]interface{}
}

type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	url      chan string
	holdable *heldListener
	logger   logger.Logger

	// mu controls access to both status and reporter.
	mu     sync.Mutex
	status string
}

// Kill implements worker.Kill.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Wait.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	w.mu.Lock()
	result := map[string]interface{}{
		"api-port": w.config.APIPort,
		"status":   w.status,
	}
	if w.holdable != nil {
		result["ports"] = w.holdable.report()
	}
	if w.config.ControllerAPIPort != 0 {
		result["api-port-open-delay"] = w.config.APIPortOpenDelay
		result["controller-api-port"] = w.config.ControllerAPIPort
	}
	w.mu.Unlock()
	return result
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
	ctx, cancel := w.scopedContext()
	defer cancel()

	serverLog := log.New(&loggerWrapper{
		level:  logger.WARNING,
		logger: w.logger,
	}, "", 0) // no prefix and no flags so log.Logger doesn't add extra prefixes
	server := &http.Server{
		Handler:   w.config.Mux,
		TLSConfig: w.config.TLSConfig,
		ErrorLog:  serverLog,
	}
	go func() {
		err := server.Serve(tls.NewListener(w.holdable, w.config.TLSConfig))
		if err != nil && err != http.ErrServerClosed {
			w.logger.Errorf(ctx, "server finished with error %v", err)
		}
	}()
	defer func() {
		// Release the holdable listener to unblock any pending accepts.
		// This needs to be done before asking to server to shutdown since
		// starting in Go 1.16, the server will wait for all listeners to
		// close before exiting.
		w.holdable.release()
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		err := server.Shutdown(ctx)
		w.catacomb.Kill(err)
	}()

	w.mu.Lock()
	w.status = "running"
	w.mu.Unlock()

	for {
		select {
		case <-w.catacomb.Dying():
			w.mu.Lock()
			w.status = "dying"
			w.mu.Unlock()
			// Stop accepting new connections. This allows the mux
			// to process all pending requests without having to deal with
			// new ones.
			w.holdable.hold()
			return w.shutdown(ctx)
		case w.url <- w.holdable.URL():
		}
	}
}

func (w *Worker) shutdown(ctx context.Context) error {
	muxDone := make(chan struct{})
	go func() {
		// Asked to shutdown - make sure we wait until all clients
		// have finished up.
		w.config.Mux.Wait()
		close(muxDone)
	}()
	select {
	case <-muxDone:
	case <-w.config.Clock.After(w.config.MuxShutdownWait):
		msg := "timeout waiting for apiserver shutdown"
		dumpFile, err := w.dumpDebug()
		if err == nil {
			w.logger.Warningf(ctx, "%v\ndebug info written to %v", msg, dumpFile)
		} else {
			w.logger.Warningf(ctx, "%v\nerror writing debug info: %v", msg, err)
		}
	}
	return w.catacomb.ErrDying()
}

func (w *Worker) dumpDebug() (string, error) {
	dumpFile, err := os.OpenFile(filepath.Join(w.config.LogDir, "apiserver-debug.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer dumpFile.Close()
	if _, err = io.WriteString(dumpFile, fmt.Sprintf("goroutine dump %v\n", time.Now().Format(time.RFC3339))); err != nil {
		return "", errors.Annotate(err, "writing header to apiserver log file")
	}
	return dumpFile.Name(), pprof.Lookup("goroutine").WriteTo(dumpFile, 1)
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

type heldListener struct {
	listener
	clock clock.Clock
	cond  *sync.Cond
	held  bool
}

func newHeldListener(l listener, c clock.Clock) *heldListener {
	var mu sync.Mutex
	return &heldListener{
		listener: l,
		clock:    c,
		cond:     sync.NewCond(&mu),
	}
}

func (h *heldListener) report() map[string]interface{} {
	result := h.listener.report()
	h.cond.L.Lock()
	if h.held {
		result["held"] = true
	}
	h.cond.L.Unlock()
	return result
}

func (h *heldListener) hold() {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	h.held = true
	// No need to signal the cond here, since nothing that's waiting
	// for the listener to be unheld can run.
}

func (h *heldListener) release() {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	h.held = false
	// Wake up any goroutines waiting for held to be false.
	h.cond.Broadcast()
}

func (h *heldListener) Accept() (net.Conn, error) {
	h.cond.L.Lock()
	wasHeld := false
	for h.held {
		wasHeld = true
		h.cond.Wait()
	}
	h.cond.L.Unlock()
	// If we were held pending a shutdown,
	// do not accept any connections.
	if wasHeld {
		return nil, http.ErrServerClosed
	}
	return h.listener.Accept()
}

type listener interface {
	net.Listener
	reporter
	URL() string
}

func (w *Worker) newSimpleListener() (listener, error) {
	listenAddr := net.JoinHostPort("", strconv.Itoa(w.config.APIPort))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.logger.Infof(context.Background(), "listening on %q", listener.Addr())
	return &simpleListener{listener}, nil
}

type simpleListener struct {
	net.Listener
}

func (s *simpleListener) URL() string {
	return fmt.Sprintf("https://%s", s.Addr())
}

func (s *simpleListener) report() map[string]interface{} {
	return map[string]interface{}{
		"listening": s.Addr().String(),
	}
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
	if err != nil {
		return nil, err
	}
	w.logger.Infof(context.Background(), "listening for controller connections on %q", listener.Addr())
	dual := &dualListener{
		agentName:          w.config.AgentName,
		clock:              w.config.Clock,
		delay:              w.config.APIPortOpenDelay,
		apiPort:            w.config.APIPort,
		controllerListener: listener,
		status:             "waiting for signal to open agent port",
		done:               make(chan struct{}),
		errors:             make(chan error),
		connections:        make(chan net.Conn),
		logger:             w.logger,
	}
	go dual.accept(listener)

	dual.unsub, err = w.config.Hub.Subscribe(apiserver.ConnectTopic, dual.openAPIPort)
	if err != nil {
		dual.Close()
		return nil, errors.Annotate(err, "unable to subscribe to details topic")
	}

	return dual, err
}

type dualListener struct {
	agentName string
	clock     clock.Clock
	delay     time.Duration
	apiPort   int

	controllerListener net.Listener
	apiListener        net.Listener
	status             string

	mu     sync.Mutex
	closer sync.Once

	done        chan struct{}
	errors      chan error
	connections chan net.Conn

	logger logger.Logger

	unsub func()
}

func (d *dualListener) report() map[string]interface{} {
	result := map[string]interface{}{
		"controller": d.controllerListener.Addr().String(),
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != "" {
		result["status"] = d.status
	}
	if d.apiListener != nil {
		result["agent"] = d.apiListener.Addr().String()
	}
	return result
}

func (d *dualListener) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case d.errors <- err:
			case <-d.done:
				d.logger.Infof(context.Background(), "no longer accepting connections on %s", listener.Addr())
				return
			}
		} else {
			select {
			case d.connections <- conn:
			case <-d.done:
				conn.Close()
				d.logger.Infof(context.Background(), "no longer accepting connections on %s", listener.Addr())
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
		// Don't wrap this error with errors.Trace - the stdlib http
		// server code has handling for various net error types (like
		// temporary failures) that we don't want to interfere with.
		return nil, err
	case conn := <-d.connections:
		// Due to the non-deterministic nature of select, it is possible
		// that if there was a pending accept call we may get a connection
		// even though we are done. So check that before we return
		// the conn.
		select {
		case <-d.done:
			conn.Close()
			return nil, errors.New("listener has been closed")
		default:
			return conn, nil
		}
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
	d.status = "closed ports"
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
func (d *dualListener) openAPIPort(topic string, conn apiserver.APIConnection, err error) {
	if err != nil {
		d.logger.Errorf(context.Background(), "programming error: %v", err)
		return
	}
	// We are wanting to make sure that the api-caller has connected before we
	// open the api port. Each api connection is published with the origin tag.
	// Any origin that matches our agent name means that someone has connected
	// to us. We need to also check which agent connected as it is possible that
	// one of the other HA controller could connect before we connect to
	// ourselves.
	if conn.Origin != d.agentName || conn.AgentTag != d.agentName {
		return
	}

	d.unsub()
	if d.delay > 0 {
		d.mu.Lock()
		d.status = "waiting prior to opening agent port"
		d.mu.Unlock()
		d.logger.Infof(context.Background(), "waiting for %s before allowing api connections", d.delay)
		select {
		case <-d.done:
			// while waiting, we were asked to shut down
			d.logger.Debugf(context.Background(), "shutting down API port before opening")
			return
		case <-d.clock.After(d.delay):
			// We are all good.
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	// Make sure we haven't been closed already.
	select {
	case <-d.done:
		d.logger.Infof(context.Background(), "shutting down API port before allowing connections", d.delay)
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
			d.logger.Errorf(context.Background(), "can't open api port: %v, but worker exiting already", err)
		}
		return
	}

	d.logger.Infof(context.Background(), "listening for api connections on %q", listener.Addr())
	d.apiListener = listener
	go d.accept(listener)
	d.status = ""
}
