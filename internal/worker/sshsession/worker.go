// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	gossh "golang.org/x/crypto/ssh"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/sshconn"
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

	// ControllerSSHPort returns the port the controller SSH jump server listens
	// on.
	ControllerSSHPort(ctx context.Context) (int, error)

	// ControllerPublicKey returns the marshalled public host key of the
	// controller SSH jump server, used to pin the host key when reverse-dialling.
	ControllerPublicKey(ctx context.Context) ([]byte, error)
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
	// DialController reverse-dials the controller SSH server and opens the
	// tunnel channel.
	DialController(ctx context.Context, address string, port int, username, password string, hostPublicKey gossh.PublicKey) (HalfCloseConn, error)
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
	// Fetch the controller SSH port and host public key once. These are stable
	// per controller HA identity, and are used to reverse-dial and pin the host
	// key for every connection request this worker handles.
	controllerSSHPort, err := w.config.FacadeClient.ControllerSSHPort(ctx)
	if err != nil {
		return errors.Errorf("getting controller SSH port: %w", err)
	}
	marshalledHostKey, err := w.config.FacadeClient.ControllerPublicKey(ctx)
	if err != nil {
		return errors.Errorf("getting controller public key: %w", err)
	}
	controllerHostPublicKey, err := gossh.ParsePublicKey(marshalledHostKey)
	if err != nil {
		return errors.Errorf("parsing controller public key: %w", err)
	}

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
				w.handleConnection(ctx, tunnelID, controllerSSHPort, controllerHostPublicKey)
			}
		}
	}
}

// handleConnection handles a single connection request in its own
// goroutine. The handler uses the worker-scoped context, so it is cancelled
// when the worker is dying, and is tracked by the worker's WaitGroup so it
// drains on shutdown. A single failed request must not bring down the worker.
func (w *sshSessionWorker) handleConnection(ctx context.Context, tunnelID string, controllerSSHPort int, controllerHostPublicKey gossh.PublicKey) {
	w.wg.Go(func() {
		if err := w.handleConnectionInternal(ctx, tunnelID, controllerSSHPort, controllerHostPublicKey); err != nil {
			w.config.Logger.Errorf(ctx, "failed to handle SSH connection request %q: %v", tunnelID, err)
		}
	})
}

// handleConnectionInternal reads the request and, if it targets this machine,
// establishes the reverse tunnel.
func (w *sshSessionWorker) handleConnectionInternal(ctx context.Context, tunnelID string, controllerSSHPort int, controllerHostPublicKey gossh.PublicKey) error {
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

	// Reverse-dial the originating controller for origin-controller affinity.
	address := req.ControllerAddresses[0]
	return w.pipeConnectionToSSHD(ctx, address, controllerSSHPort, req.Username, req.Password, controllerHostPublicKey)
}

// pipeConnectionToSSHD reverse-dials the controller and pipes the tunnel to the
// local sshd. It blocks until the connection finishes or the context is done.
func (w *sshSessionWorker) pipeConnectionToSSHD(
	ctx context.Context,
	address string,
	port int,
	username string,
	password string,
	hostPublicKey gossh.PublicKey,
) error {
	controllerConn, err := w.config.ConnectionDialer.DialController(ctx, address, port, username, password, hostPublicKey)
	if err != nil {
		return errors.Errorf("dialling controller %s:%d: %w", address, port, err)
	}
	defer func() { _ = controllerConn.Close() }()

	sshdConn, err := w.config.ConnectionDialer.DialLocalSSHD(ctx)
	if err != nil {
		return errors.Errorf("dialling local sshd: %w", err)
	}
	defer func() { _ = sshdConn.Close() }()

	stop := context.AfterFunc(ctx, func() {
		_ = controllerConn.Close()
		_ = sshdConn.Close()
	})
	defer stop()

	// Copy data in both directions between the controller tunnel and the local
	// sshd. Each direction is half-closed (CloseWrite) rather than hard-closed
	// when it finishes, so the peer receives a clean end-of-stream marker (a TCP
	// FIN on the sshd connection, an SSH channel-EOF on the controller
	// connection) and can flush any remaining data before the tunnel is torn
	// down, instead of being reset mid-stream.
	//
	// For example, on an `exit` command: the client's EOF arrives on the controller
	// connection, so the controller->sshd copy CloseWrites sshd; sshd then exits,
	// flushes its final output and closes; the sshd->controller copy drains that
	// output and CloseWrites the controller connection. The deferred Close calls
	// above (and the context.AfterFunc on cancellation) perform the full teardown
	// once both directions have finished.
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

// connectionDialer is the default ConnectionDialer. It reverse-dials the
// controller SSH server and dials the local sshd.
type connectionDialer struct {
	logger          logger.Logger
	sshdConfigPaths []string
}

// newConnectionDialer returns a new connectionDialer.
func newConnectionDialer(l logger.Logger) *connectionDialer {
	return &connectionDialer{
		logger:          l,
		sshdConfigPaths: coressh.DefaultSSHDConfigPaths,
	}
}

// DialController reverse-dials the controller SSH server and opens the tunnel
// channel.
func (d *connectionDialer) DialController(
	ctx context.Context,
	address string,
	port int,
	username string,
	password string,
	hostPublicKey gossh.PublicKey,
) (HalfCloseConn, error) {
	sshConfig := &gossh.ClientConfig{
		User:            username,
		Auth:            []gossh.AuthMethod{gossh.Password(password)},
		HostKeyCallback: gossh.FixedHostKey(hostPublicKey),
		Timeout:         controllerDialTimeout,
	}

	client, err := gossh.Dial("tcp", net.JoinHostPort(address, strconv.Itoa(port)), sshConfig)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ch, reqs, err := client.OpenChannel(coressh.JujuTunnelChannel, nil)
	if err != nil {
		_ = client.Close()
		return nil, errors.Capture(err)
	}
	go gossh.DiscardRequests(reqs)

	return sshconn.NewChannelConn(ch), nil
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
