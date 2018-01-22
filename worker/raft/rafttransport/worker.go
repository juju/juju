// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/replicaset"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	pubsubapiserver "github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/raft/raftutil"
)

var (
	logger = loggo.GetLogger("juju.worker.raft.rafttransport")
)

const (
	maxPoolSize = replicaset.MaxPeers
)

// Config is the configuration required for running an apiserver-based
// raft transport worker.
type Config struct {
	// APIInfo contains the information, excluding addresses,
	// required to connect to an API server.
	APIInfo *api.Info

	// APIOpen is the function used to dial an API connection.
	APIOpen api.OpenFunc

	// Hub is the structured hub to which the worker will subscribe
	// for API server address updates.
	Hub *pubsub.StructuredHub

	// Mux is the API server HTTP mux into which the handler will
	// be installed.
	Mux *apiserverhttp.Mux

	// Path is the path of the raft HTTP endpoint.
	Path string

	// Tag is the tag of the agent running this worker.
	Tag names.Tag

	// Timeout, if non-zero, is the timeout to apply to transport
	// operations. See raft.NetworkTransportConfig.Timeout for more
	// details.
	Timeout time.Duration
}

// Validate validates the raft worker configuration.
func (config Config) Validate() error {
	if config.APIInfo == nil {
		return errors.NotValidf("nil APIInfo")
	}
	if config.APIOpen == nil {
		return errors.NotValidf("nil APIOpen")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.Mux == nil {
		return errors.NotValidf("nil Mux")
	}
	if config.Path == "" {
		return errors.NotValidf("empty Path")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil Tag")
	}
	return nil
}

// NewWorker returns a new apiserver-based raft transport worker,
// with the given configuration. The worker itself implements
// raft.Transport.
func NewWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config:         config,
		connections:    make(chan net.Conn),
		dialRequests:   make(chan dialRequest),
		serversUpdated: make(chan struct{}),
	}

	const logPrefix = "[transport] "
	logWriter := &raftutil.LoggoWriter{logger, loggo.DEBUG}
	logLogger := log.New(logWriter, logPrefix, 0)
	transport := raft.NewNetworkTransportWithConfig(&raft.NetworkTransportConfig{
		Logger:                logLogger,
		MaxPool:               maxPoolSize,
		ServerAddressProvider: serverAddressProvider{},
		Stream: newStreamLayer(config.Tag, w.connections, &Dialer{
			APIInfo: config.APIInfo,
			DialRaw: w.dialRaw,
			Path:    config.Path,
		}),
		Timeout: config.Timeout,
	})
	w.Transport = transport

	// Subscribe to API server address changes.
	unsubscribeHub, err := w.config.Hub.Subscribe(
		pubsubapiserver.DetailsTopic,
		w.apiserverDetailsChanged,
	)
	if err != nil {
		return nil, errors.Annotate(err, "subscribing to apiserver details")
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			defer transport.Close()
			defer unsubscribeHub()
			return w.loop()
		},
	}); err != nil {
		transport.Close()
		unsubscribeHub()
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker is a worker that manages a raft.Transport.
type Worker struct {
	raft.Transport

	catacomb     catacomb.Catacomb
	config       Config
	connections  chan net.Conn
	dialRequests chan dialRequest

	mu             sync.Mutex
	servers        pubsubapiserver.Details
	serversUpdated chan struct{}
}

type dialRequest struct {
	ctx    context.Context
	tag    names.MachineTag
	result chan<- dialResult
}

type dialResult struct {
	conn net.Conn
	err  error
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// dialRaw dials a new TLS connection to the controller identified
// by the given address. The address is expected to be the stringified
// tag of a controller machine agent. The resulting connection is
// appropriate for use as Dialer.DialRaw.
func (w *Worker) dialRaw(address raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	tag, err := names.ParseMachineTag(string(address))
	if err != nil {
		return nil, net.InvalidAddrError(err.Error())
	}

	// Give precedence to the worker dying.
	select {
	case <-w.catacomb.Dying():
		return nil, w.errDialWorkerStopped()
	default:
	}

	ctx := context.Background()
	if timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	resultCh := make(chan dialResult)
	req := dialRequest{
		ctx:    ctx,
		tag:    tag,
		result: resultCh,
	}
	select {
	case <-w.catacomb.Dying():
		return nil, w.errDialWorkerStopped()
	case <-ctx.Done():
		return nil, dialRequestTimeoutError{}
	case w.dialRequests <- req:
	}

	select {
	case res := <-resultCh:
		return res.conn, res.err
	case <-ctx.Done():
		return nil, dialRequestTimeoutError{}
	case <-w.catacomb.Dying():
		return nil, w.errDialWorkerStopped()
	}
}

func (w *Worker) errDialWorkerStopped() error {
	err := w.catacomb.Err()
	if err != nil && err != w.catacomb.ErrDying() {
		return dialWorkerStoppedError{err}
	}
	return dialWorkerStoppedError{
		errors.New("worker stopped"),
	}
}

func (w *Worker) loop() error {
	h := NewHandler(w.connections, w.catacomb.Dying())
	w.config.Mux.AddHandler("GET", w.config.Path, &ControllerHandler{
		Mux:     w.config.Mux,
		Handler: h,
	})
	defer w.config.Mux.RemoveHandler("GET", w.config.Path)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case req := <-w.dialRequests:
			go w.handleDial(req)
		}
	}
}

func (w *Worker) apiserverDetailsChanged(topic string, details pubsubapiserver.Details, err error) {
	if err != nil {
		// This should never happen, so treat it as fatal.
		w.catacomb.Kill(errors.Annotate(err, "apiserver details callback failed"))
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.servers = details
	close(w.serversUpdated)
	w.serversUpdated = make(chan struct{})
}

func (w *Worker) handleDial(req dialRequest) {
	addrs := w.serverAddresses(req.ctx, req.tag)
	if addrs == nil {
		// context expired or canceled
		return
	}

	dial := func() (net.Conn, error) {
		// Dial an api.Connection first, so we get the
		// right address to connect to. The multi-address
		// connection logic should be refactored so we
		// can do without this.
		apiInfo := *w.config.APIInfo
		apiInfo.SkipLogin = true
		apiInfo.Tag = nil
		apiInfo.Password = ""
		apiInfo.Macaroons = nil
		apiInfo.Addrs = addrs
		dialOpts := api.DefaultDialOpts()
		if deadline, ok := req.ctx.Deadline(); ok {
			dialOpts.Timeout = time.Until(deadline)
		}
		apiConn, err := w.config.APIOpen(&apiInfo, dialOpts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer apiConn.Close()
		return apiConn.DialConn(req.ctx)
	}

	conn, err := dial()
	select {
	case req.result <- dialResult{conn, err}:
		return
	case <-req.ctx.Done():
	case <-w.catacomb.Dying():
	}
	if err == nil {
		// result wasn't delivered, close connection
		conn.Close()
	}
}

func (w *Worker) serverAddresses(ctx context.Context, tag names.MachineTag) []string {
	for {
		w.mu.Lock()
		server, ok := w.servers.Servers[tag.Id()]
		serversUpdated := w.serversUpdated
		w.mu.Unlock()
		if ok {
			return server.Addresses
		}
		logger.Tracef("waiting for address for %s", tag)
		select {
		case <-serversUpdated:
			continue
		case <-ctx.Done():
		case <-w.catacomb.Dying():
		}
		// context expired or canceled
		return nil
	}
}

// serverAddressProvider is an implementation of raft.ServerAddressProvider
// that returns the provided server ID as the address, verbatim, so long as
// it is valid machine tag.
type serverAddressProvider struct{}

func (serverAddressProvider) ServerAddr(id raft.ServerID) (raft.ServerAddress, error) {
	if _, err := names.ParseMachineTag(string(id)); err != nil {
		return "", err
	}
	return raft.ServerAddress(string(id)), nil
}
