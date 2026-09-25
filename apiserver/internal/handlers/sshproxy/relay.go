// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"net/http"

	"github.com/lestrrat-go/jwx/v3/jwt"
	"gopkg.in/tomb.v2"

	authjwt "github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	coresshproxy "github.com/juju/juju/core/sshproxy"
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
//
// The handler is a worker: the apiserver attaches it to its catacomb,
// so killing it cancels the request contexts of in-flight relays. Each
// relay watches its request context and closes its hijacked connection
// when the handler dies, draining the relays on apiserver shutdown
// (hijacked connections are invisible to http.Server.Shutdown).
type RelayHandler struct {
	tomb   tomb.Tomb
	config RelayHandlerConfig
}

// RelayHandlerConfig holds the configuration for the JIMM relay endpoint.
type RelayHandlerConfig struct {
	// Logger is used for logging.
	Logger logger.Logger
	// ServerFactory builds the per-destination terminating SSH server.
	ServerFactory coresshproxy.TerminatingServerFactory
}

// Validate checks whether the configuration is valid.
func (cfg RelayHandlerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.New("nil Logger")
	}
	if cfg.ServerFactory == nil {
		return errors.New("nil ServerFactory")
	}
	return nil
}

// NewRelayHandler returns a new JIMM relay endpoint handler.
func NewRelayHandler(config RelayHandlerConfig) (*RelayHandler, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating relay handler config: %w", err)
	}
	h := &RelayHandler{config: config}
	// Without a tracked goroutine the tomb never reaches dead, so Wait
	// would block forever.
	h.tomb.Go(func() error {
		<-h.tomb.Dying()
		return tomb.ErrDying
	})
	return h, nil
}

// Kill implements worker.Worker.Kill. It cancels the request contexts of
// in-flight relays, whose dying watches close their connections.
func (h *RelayHandler) Kill() {
	h.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait. It blocks until the handler is
// killed. In-flight relays observe the cancellation through their request
// contexts and close their connections.
func (h *RelayHandler) Wait() error {
	return h.tomb.Wait()
}

// ServeHTTP implements http.Handler.
func (h *RelayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The request context is cancelled when the handler's tomb starts
	// dying, so in-flight relays observe shutdown and their connections
	// are closed by the tomb goroutine.
	ctx := h.tomb.Context(r.Context())
	r = r.WithContext(ctx)

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

	conn, err := hijack(w, r, coresshproxy.RelayUpgradeToken)
	if err != nil {
		h.config.Logger.Errorf(ctx, "upgrading relay connection: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	// HandleConn blocks for the life of the SSH session and cannot be
	// cancelled, so watch the request context and close the connection
	// when the handler dies. The watch goroutine exits when the session
	// ends, so it does not outlive the connection.
	sessionDone := make(chan struct{})
	defer close(sessionDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-sessionDone:
		}
	}()

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
