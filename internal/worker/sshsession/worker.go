// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
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

// ConnectionDialer establishes the controller and local sshd connections that
// the worker pipes together to form a reverse tunnel.
type ConnectionDialer interface {
	// DialController reverse-dials the controller SSH server and opens the
	// tunnel channel, returning it as a net.Conn.
	DialController(ctx context.Context, address string, port int, username, password string, hostPublicKey gossh.PublicKey) (net.Conn, error)
	// DialLocalSSHD dials the local sshd.
	DialLocalSSHD(ctx context.Context) (net.Conn, error)
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
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := w.handleConnectionInternal(ctx, tunnelID, controllerSSHPort, controllerHostPublicKey); err != nil {
			w.config.Logger.Errorf(ctx, "failed to handle SSH connection request %q: %v", tunnelID, err)
		}
	}()
}

// handleConnectionInternal reads the request and, if it targets this machine,
// establishes the reverse tunnel.
func (w *sshSessionWorker) handleConnectionInternal(ctx context.Context, tunnelID string, controllerSSHPort int, controllerHostPublicKey gossh.PublicKey) error {
	req, err := w.config.FacadeClient.GetSSHConnRequest(ctx, tunnelID)
	if err != nil {
		return errors.Errorf("getting SSH connection request %q: %w", tunnelID, err)
	}

	// Requests are model-scoped and the watcher emits all of them; only handle
	// those targeting this machine.
	if req.MachineName != w.config.MachineName {
		w.config.Logger.Tracef(ctx, "ignoring SSH connection request %q for machine %q", tunnelID, req.MachineName)
		return nil
	}

	if len(req.ControllerAddresses) == 0 {
		return errors.Errorf("SSH connection request %q has no controller addresses", tunnelID)
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

	// Close both connections when the context is done, otherwise signal the
	// goroutine to exit once the copies finish.
	doneChan := make(chan struct{})
	defer close(doneChan)
	go func() {
		select {
		case <-ctx.Done():
			_ = controllerConn.Close()
			_ = sshdConn.Close()
		case <-doneChan:
		}
	}()

	var wg sync.WaitGroup
	wg.Go(func() {
		defer controllerConn.Close()
		defer sshdConn.Close()
		_, _ = io.Copy(sshdConn, controllerConn)
	})
	wg.Go(func() {
		defer controllerConn.Close()
		defer sshdConn.Close()
		_, _ = io.Copy(controllerConn, sshdConn)
	})
	wg.Wait()
	return nil
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
		sshdConfigPaths: []string{"/etc/ssh/sshd_config", "/usr/share/openssh/sshd_config"},
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
) (net.Conn, error) {
	sshConfig := &gossh.ClientConfig{
		User:            username,
		Auth:            []gossh.AuthMethod{gossh.Password(password)},
		HostKeyCallback: gossh.FixedHostKey(hostPublicKey),
		Timeout:         controllerDialTimeout,
	}

	client, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", address, port), sshConfig)
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
func (d *connectionDialer) DialLocalSSHD(ctx context.Context) (net.Conn, error) {
	port := d.localSSHPort(ctx)
	conn, err := net.Dial("tcp", net.JoinHostPort("localhost", port))
	if err != nil {
		return nil, errors.Capture(err)
	}
	return conn, nil
}

// localSSHPort parses the local sshd_config files to find the port sshd is
// listening on, trying each configured path. If it cannot be determined, it
// logs the error and returns the default port 22.
func (d *connectionDialer) localSSHPort(ctx context.Context) string {
	const defaultPort = "22"

	for _, filePath := range d.sshdConfigPaths {
		file, err := os.Open(filePath)
		if err != nil {
			d.logger.Errorf(ctx, "opening sshd_config file %q: %v", filePath, err)
			continue
		}
		port := defaultPort
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			if strings.HasPrefix(line, "Port") {
				fields := strings.Fields(line)
				if len(fields) == 2 {
					port = fields[1]
				}
				break
			}
		}
		if err := scanner.Err(); err != nil {
			d.logger.Errorf(ctx, "reading sshd_config file %q: %v", filePath, err)
		}
		_ = file.Close()
		return port
	}

	return defaultPort
}
