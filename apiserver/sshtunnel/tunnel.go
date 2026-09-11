// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunnel

import (
	"context"
	"net"
	"net/http"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
)

// TunnelTracker accepts reverse tunnel connections pushed by machine
// agents. It is the same interface exposed by the sshtunneler worker.
type TunnelTracker interface {
	// PushTunnel publishes an established network connection for the tunnel
	// identified by tunnelID. The tunnel ID must have been created by a
	// RequestTunnel call on this controller node. The returned channel is
	// closed when the pushed connection is closed, so the caller can
	// select on it to detect tunnel teardown.
	PushTunnel(ctx context.Context, tunnelID string, conn net.Conn) (<-chan struct{}, error)
}

// SSHConnRequestService reads one-shot SSH connection requests from
// model-scoped state.
type SSHConnRequestService interface {
	// GetSSHConnRequest returns the SSH connection request for the supplied
	// tunnel ID, scoped to the named machine. A request targeting another
	// machine is reported as not found.
	GetSSHConnRequest(ctx context.Context, machineName string, tunnelID string) (SSHConnRequest, error)
}

// SSHConnRequest is the subset of the domain SSH connection request the
// handler needs: the machine the request targets.
type SSHConnRequest struct {
	// MachineName is the name of the machine the request targets.
	MachineName string
}

// TunnelHandlerConfig holds the configuration for the agent tunnel endpoint.
type TunnelHandlerConfig struct {
	// Logger is used for logging.
	Logger logger.Logger
	// Tracker is the local controller node's tunnel tracker. Tunnel IDs
	// created on other nodes are rejected, preserving origin controller
	// affinity.
	Tracker TunnelTracker
	// SSHConnRequestService reads the connection request to bind the
	// tunnel ID to the authenticated machine.
	SSHConnRequestService SSHConnRequestService
	// Metrics collects connection metrics, reusing the sshserver collector
	// so the upgrade path is accounted the same as the SSH server was.
	Metrics MetricsCollector
}

// Validate checks whether the configuration is valid.
func (cfg TunnelHandlerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.New("nil Logger")
	}
	if cfg.Tracker == nil {
		return errors.New("nil Tracker")
	}
	if cfg.SSHConnRequestService == nil {
		return errors.New("nil SSHConnRequestService")
	}
	if cfg.Metrics == nil {
		return errors.New("nil Metrics")
	}
	return nil
}

// MetricsCollector is the subset of the sshserver metrics collector used by
// the tunnel endpoint.
type MetricsCollector interface {
	// IncConnectionCount increments the active connection count.
	IncConnectionCount()
	// DecConnectionCount decrements the active connection count and records
	// the connection duration.
	DecConnectionCount()
}

// TunnelHandler implements the model-scoped agent tunnel upgrade endpoint:
//
//	GET /model/:modeluuid/ssh-tunnel/:tunnelID
//
// A machine agent dials this endpoint on a new HTTPS connection to one of
// the controller addresses from its SSH connection request. Authentication
// happens at the HTTP layer (model-scoped agent-password login, the same
// mechanism as /logsink), and the handler binds the tunnel ID to the
// authenticated machine: the request must target that machine and the
// tunnel ID must be known to this node's tracker. After the upgrade the
// connection carries raw bytes; the tracker SSH-dials the machine over it
// with the ephemeral key.
type TunnelHandler struct {
	config TunnelHandlerConfig
}

// NewTunnelHandler returns a new agent tunnel endpoint handler.
func NewTunnelHandler(config TunnelHandlerConfig) (*TunnelHandler, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating tunnel handler config: %w", err)
	}
	return &TunnelHandler{config: config}, nil
}

// ServeHTTP implements http.Handler.
func (h *TunnelHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// The authenticated machine tag comes from the HTTP authentication
	// layer; the tunnel ID alone is not sufficient to establish identity.
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

	// Bind the tunnel ID to the authenticated machine: the connection
	// request must exist and must target this machine. A request for
	// another machine's tunnel is reported as not found.
	_, err := h.config.SSHConnRequestService.GetSSHConnRequest(ctx, machineName, tunnelID)
	if err != nil {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	h.config.Metrics.IncConnectionCount()
	defer h.config.Metrics.DecConnectionCount()

	conn, err := hijack(w, r, TunnelUpgradeToken)
	if err != nil {
		h.config.Logger.Errorf(ctx, "upgrading tunnel connection: %v", err)
		return
	}
	stop := watchDying(conn, dyingFromContext(ctx), h.config.Logger)
	defer stop()

	// Push the connection to the tracker. The tracker validates the tunnel
	// ID against its local state: a request landing on the wrong controller
	// node is rejected and the machine retries the next address. The
	// returned done channel is closed when the pushed connection is closed
	// (either by the tunnel consumer or by the dying-signal watch), so the
	// handler blocks until the tunnel ends rather than until the apiserver
	// dies — preventing a goroutine leak per tunnel.
	pushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pushTunnelTimeout)
	defer cancel()
	done, err := h.config.Tracker.PushTunnel(pushCtx, tunnelID, conn)
	if err != nil {
		h.config.Logger.Errorf(ctx, "pushing tunnel %q: %v", tunnelID, err)
		_ = conn.Close()
		return
	}

	// Block until the tunnel ends (done) or the apiserver is shutting
	// down (dying). The watchDying goroutine closes the connection on
	// dying, which in turn closes the done channel and lets the handler
	// return.
	select {
	case <-done:
	case <-dyingFromContext(ctx):
	}
}

// AuthenticatedMachineNameKey is the context key for the authenticated
// machine name, set by the apiserver from the request's auth info.
type AuthenticatedMachineNameKey struct{}

// dyingFromContext extracts the apiserver dying signal from the request
// context. The apiserver sets it via the handler wrapper.
func dyingFromContext(ctx context.Context) <-chan struct{} {
	if ch, ok := ctx.Value(DyingKey{}).(<-chan struct{}); ok {
		return ch
	}
	// No dying signal in context: return nil so select blocks
	// forever, letting the tunnel run until done closes.
	return nil
}
