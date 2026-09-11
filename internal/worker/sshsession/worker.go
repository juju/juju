// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/sshtunnel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

const (
	// controllerDialTimeout bounds the reverse-dial to the controller.
	controllerDialTimeout = 30 * time.Second
	// localSSHDDialTimeout bounds the dial to the local sshd. sshd is on
	// localhost so a connection should be near-instant; the timeout ensures an
	// unreachable sshd fails fast rather than hanging the handler.
	localSSHDDialTimeout = 30 * time.Second
)

// FacadeClient holds the facade methods required by the SSH session worker.
type FacadeClient interface {
	// WatchSSHConnRequest returns a watcher that emits the tunnel IDs of SSH
	// connection requests in the model.
	WatchSSHConnRequest(ctx context.Context) (watcher.StringsWatcher, error)

	// GetSSHConnRequest returns the SSH connection request for the supplied
	// tunnel ID.
	GetSSHConnRequest(ctx context.Context, tunnelID string) (params.SSHConnRequestResult, error)
}

// HalfCloseConn is a net.Conn that additionally supports half-close: signalling
// EOF on the write side without tearing down the read side. Both connection
// ends of the reverse tunnel support this - the controller side is an SSH channel
// and the sshd side is a TCP connection - which lets each direction of the
// tunnel be closed gracefully when its copy completes.
type HalfCloseConn interface {
	net.Conn
	CloseWrite() error
}

// ConnectionDialer establishes the controller and local sshd connections that
// the worker pipes together to form a reverse tunnel.
type ConnectionDialer interface {
	// DialController opens an HTTPS connection to the controller's SSH
	// tunnel upgrade endpoint and returns the upgraded raw connection.
	// The model UUID identifies the model-scoped endpoint path; the agent
	// credentials authenticate at the HTTP layer.
	DialController(ctx context.Context, address string, modelUUID string, tunnelID string) (HalfCloseConn, error)
	// DialLocalSSHD dials the local sshd.
	DialLocalSSHD(ctx context.Context) (HalfCloseConn, error)
}

// WorkerConfig holds the configuration for a new sshsession worker.
type WorkerConfig struct {
	// Logger is used for logging.
	Logger logger.Logger
	// MachineName is the name of the machine this agent runs on. The worker
	// only handles requests targeting this machine.
	MachineName string
	// ModelUUID is the UUID of the model this agent belongs to. It scopes the
	// controller's SSH tunnel upgrade endpoint path.
	ModelUUID string
	// FacadeClient is used to watch and read SSH connection requests.
	FacadeClient FacadeClient
	// EphemeralKeysUpdater injects/removes ephemeral keys.
	EphemeralKeysUpdater coressh.EphemeralKeysUpdater
	// ConnectionDialer establishes controller and local sshd connections.
	ConnectionDialer ConnectionDialer
}

// Validate checks whether the worker configuration is valid.
func (cfg WorkerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	if cfg.MachineName == "" {
		return errors.Errorf("empty MachineName").Add(coreerrors.NotValid)
	}
	if cfg.ModelUUID == "" {
		return errors.Errorf("empty ModelUUID").Add(coreerrors.NotValid)
	}
	if cfg.FacadeClient == nil {
		return errors.Errorf("nil FacadeClient").Add(coreerrors.NotValid)
	}
	if cfg.EphemeralKeysUpdater == nil {
		return errors.Errorf("nil EphemeralKeysUpdater").Add(coreerrors.NotValid)
	}
	if cfg.ConnectionDialer == nil {
		return errors.Errorf("nil ConnectionDialer").Add(coreerrors.NotValid)
	}
	return nil
}

// sshSessionWorker is a worker that enables reverse SSH connections to a
// machine.
type sshSessionWorker struct {
	catacomb catacomb.Catacomb
	config   WorkerConfig

	// wg tracks in-flight connection handlers so they drain on shutdown.
	wg sync.WaitGroup
}

// NewWorker returns a new SSH session worker.
func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	w := &sshSessionWorker{
		config: cfg,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "ssh-session",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Kill implements worker.Worker.
func (w *sshSessionWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *sshSessionWorker) Wait() error {
	return w.catacomb.Wait()
}

// loop watches for SSH connection requests and handles those targeting this
// machine.
func (w *sshSessionWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	connRequestWatcher, err := w.config.FacadeClient.WatchSSHConnRequest(ctx)
	if err != nil {
		return errors.Errorf("watching SSH connection requests: %w", err)
	}
	if err := w.catacomb.Add(connRequestWatcher); err != nil {
		return errors.Capture(err)
	}

	// Ensure in-flight connection handlers drain before the loop returns.
	defer w.wg.Wait()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes, ok := <-connRequestWatcher.Changes():
			if !ok {
				return errors.Errorf("SSH connection request watcher closed")
			}
			for _, tunnelID := range changes {
				w.handleConnection(ctx, tunnelID)
			}
		}
	}
}

// handleConnection handles a single connection request in its own
// goroutine. The handler uses the worker-scoped context, so it is cancelled
// when the worker is dying, and is tracked by the worker's WaitGroup so it
// drains on shutdown. A single failed request must not bring down the worker.
func (w *sshSessionWorker) handleConnection(ctx context.Context, tunnelID string) {
	w.wg.Go(func() {
		if err := w.handleConnectionInternal(ctx, tunnelID); err != nil {
			w.config.Logger.Errorf(ctx, "failed to handle SSH connection request %q: %v", tunnelID, err)
		}
	})
}

// handleConnectionInternal reads the request and, if it targets this machine,
// establishes the reverse tunnel.
func (w *sshSessionWorker) handleConnectionInternal(ctx context.Context, tunnelID string) error {
	req, err := w.config.FacadeClient.GetSSHConnRequest(ctx, tunnelID)
	if err != nil {
		return errors.Errorf("getting SSH connection request %q: %w", tunnelID, err)
	}

	if len(req.ControllerAddresses) == 0 {
		return errors.Errorf("SSH connection request %q has no controller addresses", tunnelID)
	}

	if req.MachineName != w.config.MachineName {
		w.config.Logger.Tracef(ctx, "ignoring SSH connection request %q for machine %q", tunnelID, req.MachineName)
		return nil
	}

	ephemeralPublicKey, err := gossh.ParsePublicKey(req.EphemeralPublicKey)
	if err != nil {
		return errors.Errorf("parsing ephemeral public key for request %q: %w", tunnelID, err)
	}

	if err := w.config.EphemeralKeysUpdater.AddEphemeralKey(ephemeralPublicKey, tunnelID); err != nil {
		return errors.Errorf("adding ephemeral key for request %q: %w", tunnelID, err)
	}
	defer func() {
		if err := w.config.EphemeralKeysUpdater.RemoveEphemeralKey(ephemeralPublicKey); err != nil {
			w.config.Logger.Errorf(ctx, "removing ephemeral key for request %q: %v", tunnelID, err)
		}
	}()

	// Dial the originating controller for origin-controller affinity.
	// ControllerAddresses are all addresses of the single originating
	// controller node, prioritized best-first, so trying alternates on
	// failure preserves affinity while adding route resilience. A request
	// landing on the wrong node is rejected by that node's tracker and the
	// machine retries the next address.
	controllerConn, err := w.dialController(ctx, req.ControllerAddresses, w.config.ModelUUID, tunnelID)
	if err != nil {
		return errors.Errorf("dialling controller for request %q: %w", tunnelID, err)
	}
	return w.pipeConnectionToSSHD(ctx, controllerConn)
}

// dialController dials the originating controller's tunnel upgrade
// endpoint, trying each address in the supplied (prioritized) order and
// returning the first successful connection. It stops early if the
// context is cancelled.
func (w *sshSessionWorker) dialController(
	ctx context.Context,
	addresses []string,
	modelUUID string,
	tunnelID string,
) (HalfCloseConn, error) {
	var err error
	for _, address := range addresses {
		var conn HalfCloseConn
		conn, err = w.config.ConnectionDialer.DialController(ctx, address, modelUUID, tunnelID)
		if err == nil {
			return conn, nil
		}
		w.config.Logger.Debugf(ctx, "dialling controller %s failed, trying next address: %v", address, err)
		// Stop trying alternates if the worker is shutting down.
		if ctx.Err() != nil {
			break
		}
	}
	if err == nil {
		err = errors.Errorf("no controller addresses to dial")
	}
	return nil, errors.Capture(err)
}

// pipeConnectionToSSHD dials the local sshd and pipes the supplied controller
// tunnel to it. It takes ownership of controllerConn and closes it. It blocks
// until the connection finishes or the context is done.
func (w *sshSessionWorker) pipeConnectionToSSHD(
	ctx context.Context,
	controllerConn HalfCloseConn,
) error {
	defer func() { _ = controllerConn.Close() }()

	sshdConn, err := w.config.ConnectionDialer.DialLocalSSHD(ctx)
	if err != nil {
		return errors.Errorf("dialling local sshd: %w", err)
	}
	defer func() { _ = sshdConn.Close() }()

	// On context cancellation (worker shutdown), force both connections closed
	// so the bidirectionalCopy below can unblock and this pipe handler can return.
	stop := context.AfterFunc(ctx, func() {
		_ = controllerConn.Close()
		_ = sshdConn.Close()
	})
	// stop() deregisters the callback above. We defer our stop to allow the
	// bidirectionalCopy to gracefully complete. If the context is cancelled whilst
	// the copy is running, the above callback will execute and stop will return false
	// (resulting in a no-op).
	defer stop()

	bidirectionalCopy(sshdConn, controllerConn)
	return nil
}

// bidirectionalCopy pipes data in both directions between two half-closeable
// connections until both directions reach EOF.
func bidirectionalCopy(a HalfCloseConn, b HalfCloseConn) {
	var wg sync.WaitGroup
	wg.Go(func() {
		defer func() { _ = a.CloseWrite() }()
		_, _ = io.Copy(a, b)
	})
	wg.Go(func() {
		defer func() { _ = b.CloseWrite() }()
		_, _ = io.Copy(b, a)
	})
	wg.Wait()
}

// connectionDialer is the default ConnectionDialer. It opens an HTTPS
// upgrade request to the controller's SSH tunnel endpoint and dials the
// local sshd.
type connectionDialer struct {
	logger          logger.Logger
	sshdConfigPaths []string
	// apiInfo holds the agent's API credentials, used to authenticate the
	// upgrade request at the HTTP layer.
	apiInfo *api.Info
}

// newConnectionDialer returns a new connectionDialer.
func newConnectionDialer(l logger.Logger, apiInfo *api.Info) *connectionDialer {
	return &connectionDialer{
		logger:          l,
		sshdConfigPaths: coressh.DefaultSSHDConfigPaths,
		apiInfo:         apiInfo,
	}
}

// DialController opens an HTTPS connection to the controller's SSH tunnel
// upgrade endpoint and returns the upgraded raw connection. Authentication
// uses the agent's API credentials (basic auth headers, the same mechanism
// as /logsink); TLS validates the controller. The request must be
// HTTP/1.1: HTTP/2 disallows the upgrade mechanism, so the TLS config pins
// ALPN to http/1.1.
func (d *connectionDialer) DialController(
	ctx context.Context,
	address string,
	modelUUID string,
	tunnelID string,
) (HalfCloseConn, error) {
	return d.upgrade(ctx, address, modelUUID, tunnelID)
}

// upgrade performs the HTTP upgrade handshake to the tunnel endpoint and
// returns the raw connection.
func (d *connectionDialer) upgrade(
	ctx context.Context,
	address string,
	modelUUID string,
	tunnelID string,
) (HalfCloseConn, error) {
	// TLS validates the controller using the API info's CA certificate.
	// ALPN is pinned to http/1.1: HTTP/2 disallows the upgrade mechanism
	// and Hijack returns ErrNotSupported on an h2 connection.
	certPool := x509.NewCertPool()
	if d.apiInfo.CACert != "" {
		if !certPool.AppendCertsFromPEM([]byte(d.apiInfo.CACert)) {
			return nil, errors.Errorf("adding CA certificate to pool")
		}
	}
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.NextProtos = []string{"http/1.1"}

	dialer := &tls.Dialer{
		Config: tlsConfig,
		NetDialer: &net.Dialer{
			Timeout: controllerDialTimeout,
		},
	}
	rawConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, errors.Errorf("dialing controller %s: %w", address, err)
	}

	// Build the upgrade request with the agent's credentials, the same
	// basic-auth mechanism the /logsink endpoint uses.
	target := &url.URL{
		Scheme: "https",
		Host:   address,
		Path:   path.Join("/model", modelUUID, "ssh-tunnel", tunnelID),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		_ = rawConn.Close()
		return nil, errors.Errorf("building upgrade request: %w", err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", sshtunnel.TunnelUpgradeToken)
	if err := api.AuthHTTPRequest(req, d.apiInfo); err != nil {
		_ = rawConn.Close()
		return nil, errors.Errorf("authenticating upgrade request: %w", err)
	}

	conn, err := sshtunnel.PerformUpgrade(req, rawConn)
	if err != nil {
		_ = rawConn.Close()
		return nil, errors.Capture(err)
	}
	return asHalfCloseConn(conn)
}

// asHalfCloseConn guards that the upgraded connection supports half-close,
// which the tunnel pipe relies on for graceful per-direction teardown.
func asHalfCloseConn(conn net.Conn) (HalfCloseConn, error) {
	hc, ok := conn.(HalfCloseConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.Errorf("upgraded connection %T does not support half-close", conn)
	}
	return hc, nil
}

// DialLocalSSHD performs a standard TCP dial to the sshd running on the
// machine.
func (d *connectionDialer) DialLocalSSHD(ctx context.Context) (HalfCloseConn, error) {
	port := d.localSSHPort(ctx)
	// Use a context-aware, timeout-bounded dial so an unreachable sshd cannot
	// hang the handler goroutine (and, through it, worker shutdown).
	dialer := net.Dialer{Timeout: localSSHDDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("localhost", port))
	if err != nil {
		return nil, errors.Capture(err)
	}
	// A TCP connection supports half-close; guard the conversion so a change to
	// the dial target that does not support it surfaces as an error rather than
	// silently degrading the tunnel teardown.
	hc, ok := conn.(HalfCloseConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.Errorf("sshd connection %T does not support half-close", conn)
	}
	return hc, nil
}

// localSSHPort parses the local sshd_config files to find the port sshd is
// listening on, trying each configured path. If it cannot be determined, it
// logs the error and returns the default port.
func (d *connectionDialer) localSSHPort(ctx context.Context) string {
	for _, filePath := range d.sshdConfigPaths {
		cfg, err := coressh.OpenSSHDConfig(filePath)
		if err != nil {
			d.logger.Errorf(ctx, "reading sshd_config file %q: %v", filePath, err)
			continue
		}
		return cfg.Port()
	}

	return coressh.DefaultSSHDPort
}
