// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"
	"net"
	"net/http"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coresshproxy "github.com/juju/juju/core/sshproxy"
	"github.com/juju/juju/internal/errors"
)

// TunnelTracker accepts reverse tunnel connections pushed by machine
// agents. It is the same interface exposed by the sshtunneler worker.
type TunnelTracker interface {
	// PushTunnel publishes an established network connection for the tunnel
	// identified by tunnelID, on behalf of the named machine. The machine
	// name must match the machine the tunnel was requested for. The
	// returned channel is closed when the pushed connection is closed.
	PushTunnel(ctx context.Context, tunnelID, machineName string, conn net.Conn) (<-chan struct{}, error)
}

// TunnelHandlerConfig holds the configuration for the agent tunnel endpoint.
type TunnelHandlerConfig struct {
	// Logger is used for logging.
	Logger logger.Logger
	// Tracker is the local controller node's tunnel tracker. Tunnel IDs
	// created on other nodes are rejected, preserving origin controller
	// affinity.
	Tracker TunnelTracker
}

// Validate checks whether the configuration is valid.
func (cfg TunnelHandlerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.New("nil Logger")
	}
	if cfg.Tracker == nil {
		return errors.New("nil Tracker")
	}
	return nil
}

// TunnelHandler implements the model-scoped agent tunnel upgrade endpoint:
//
//	GET /model/:modeluuid/ssh-tunnel/:tunnelID
//
// A machine agent dials this endpoint on a new HTTPS connection to one of
// the controller addresses from its SSH connection request. Authentication
// happens at the HTTP layer (model-scoped agent-password login, the same
// mechanism as /logsink). The tunnel ID must be known to this node's
// tracker.
//
// The handler is a worker: it owns the hijacked tunnel connections until
// they end, so the apiserver attaches it to its catacomb. Killing the
// handler closes every live tunnel connection, draining them on apiserver
// shutdown (hijacked connections are invisible to http.Server.Shutdown).
type TunnelHandler struct {
	tomb   tomb.Tomb
	config TunnelHandlerConfig
}

// NewTunnelHandler returns a new agent tunnel endpoint handler.
func NewTunnelHandler(config TunnelHandlerConfig) (*TunnelHandler, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating tunnel handler config: %w", err)
	}
	h := &TunnelHandler{config: config}
	// Without a tracked goroutine the tomb never reaches dead, so Wait
	// would block forever.
	h.tomb.Go(func() error {
		<-h.tomb.Dying()
		return tomb.ErrDying
	})
	return h, nil
}

// Kill implements worker.Worker.Kill. It closes all live tunnel
// connections, letting the ServeHTTP calls owning them return.
func (h *TunnelHandler) Kill() {
	h.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait. It blocks until the handler is
// killed. In-flight requests observe the cancellation through their
// request context and close their tunnel connections.
func (h *TunnelHandler) Wait() error {
	return h.tomb.Wait()
}

// ServeHTTP implements http.Handler.
func (h *TunnelHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The request context is cancelled when the handler's tomb starts
	// dying, so in-flight tunnels observe shutdown and close their
	// connections.
	ctx := h.tomb.Context(r.Context())
	r = r.WithContext(ctx)

	// The authenticated machine tag comes from the HTTP authentication
	// layer. The tunnel ID alone is not sufficient to establish identity.
	machineName, ok := ctx.Value(AuthenticatedMachineNameKey{}).(string)
	if !ok || machineName == "" {
		http.Error(w, "authenticated machine not found", http.StatusUnauthorized)
		return
	}
	tunnelID := r.URL.Query().Get(":tunnelID")
	if tunnelID == "" {
		http.Error(w, "missing tunnel ID", http.StatusBadRequest)
		return
	}

	conn, err := hijack(w, r, coresshproxy.TunnelUpgradeToken)
	if err != nil {
		h.config.Logger.Errorf(ctx, "upgrading tunnel connection: %v", err)
		return
	}

	// The done channel is closed when the pushed connection is closed,
	// so the handler can block until the tunnel ends without polling.
	// The push context inherits the request context, so a dying tomb
	// cancels a pending push and closes the connection.
	pushCtx, cancel := context.WithTimeout(ctx, pushTunnelTimeout)
	defer cancel()
	done, err := h.config.Tracker.PushTunnel(pushCtx, tunnelID, machineName, conn)
	if err != nil {
		h.config.Logger.Errorf(ctx, "pushing tunnel %q: %v", tunnelID, err)
		_ = conn.Close()
		return
	}

	// Block until the tunnel ends or the handler dies. conn is hijacked,
	// so we close it explicitly. Close is idempotent, so racing is
	// harmless.
	select {
	case <-done:
	case <-ctx.Done():
		_ = conn.Close()
	}
}

// AuthenticatedMachineNameKey is the context key for the authenticated
// machine name, set by the apiserver from the request's auth info.
type AuthenticatedMachineNameKey struct{}
