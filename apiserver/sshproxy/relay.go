// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"
	"net/http"

	"github.com/lestrrat-go/jwx/v3/jwt"
	ssh "github.com/tailscale/gliderssh"

	authjwt "github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/errors"
	coressh "github.com/juju/juju/internal/ssh"
)

// RelayHandler implements the JIMM relay upgrade endpoint:
//
//	GET /ssh-relay/:virtualHostname
//
// JIMM authenticates with a bearer JWT in the Authorization header (the
// same external-auth flow the API server already supports on HTTP
// endpoints). After the upgrade, the user's SSH session - relayed blind by
// JIMM - terminates in the embedded SSH server built here.
type RelayHandler struct {
	config RelayHandlerConfig
}

// RelayHandlerConfig holds the configuration for the JIMM relay endpoint.
type RelayHandlerConfig struct {
	// Logger is used for logging.
	Logger logger.Logger
	// ServerFactory builds the per-destination terminating SSH server.
	ServerFactory TerminatingServerFactory
	// Metrics collects connection metrics.
	Metrics MetricsCollector
}

// Validate checks whether the configuration is valid.
func (cfg RelayHandlerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.New("nil Logger")
	}
	if cfg.ServerFactory == nil {
		return errors.New("nil ServerFactory")
	}
	if cfg.Metrics == nil {
		return errors.New("nil Metrics")
	}
	return nil
}

// TerminatingServerFactory builds terminating SSH servers for routed
// destinations. It is satisfied by the SSH server worker's factory, whose
// output the apiserver manifold consumes.
type TerminatingServerFactory interface {
	// New returns a terminating SSH server for the destination, with proxy
	// handlers and the destination's host key configured.
	New(ctx context.Context, destination virtualhostname.Info) (*ssh.Server, error)
}

// NewRelayHandler returns a new JIMM relay endpoint handler.
func NewRelayHandler(config RelayHandlerConfig) (*RelayHandler, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating relay handler config: %w", err)
	}
	return &RelayHandler{config: config}, nil
}

// ServeHTTP implements http.Handler.
func (h *RelayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	virtualHostname := r.URL.Query().Get(":virtualHostname")
	destination, err := virtualhostname.Parse(virtualHostname)
	if err != nil {
		http.Error(w, "failed to parse destination hostname", http.StatusBadRequest)
		return
	}

	// The user identity comes from the JWT claims (PermissionDelegator
	// flow). The token was validated by the HTTP authentication layer.
	token, ok := ctx.Value(RelayJWTKey{}).(jwt.Token)
	if !ok || token == nil {
		http.Error(w, "missing relay JWT", http.StatusUnauthorized)
		return
	}

	// Authorize the destination model using the verified JWT's access
	// claims. Only admin access on the target model permits relay.
	access, err := authjwt.PermissionFromToken(token, permission.ID{
		ObjectType: permission.Model,
		Key:        destination.ModelUUID().String(),
	})
	if err != nil {
		h.config.Logger.Errorf(ctx, "authorizing relay access: %v", err)
		http.Error(w, "failed to authorize access to destination", http.StatusInternalServerError)
		return
	}
	if !access.EqualOrGreaterModelAccessThan(permission.AdminAccess) {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}

	conn, err := hijack(w, r, RelayUpgradeToken)
	if err != nil {
		h.config.Logger.Errorf(ctx, "upgrading relay connection: %v", err)
		return
	}
	h.config.Metrics.IncConnectionCount("relay")
	defer h.config.Metrics.DecConnectionCount("relay")
	stop := watchDying(conn, dyingFromContext(ctx), h.config.Logger)
	defer stop()
	defer func() { _ = conn.Close() }()

	// Resolve after the upgrade. Failures reach the user's client as SSH
	// pre-banner text through JIMM's blind relay.
	server, err := h.config.ServerFactory.New(ctx, destination)
	if err != nil {
		h.config.Logger.Errorf(ctx, "resolving destination: %v", err)
		if werr := coressh.WritePreBannerError(conn, "juju ssh relay: "+err.Error()); werr != nil {
			h.config.Logger.Errorf(ctx, "writing resolve error to connection: %v", werr)
		}
		return
	}

	server.HandleConn(conn)
}

// RelayJWTKey is the context key for the relay JWT, set by the apiserver
// from the request's auth info.
type RelayJWTKey struct{}

// DyingKey is the context key for the apiserver dying signal.
type DyingKey struct{}
