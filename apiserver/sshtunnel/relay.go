// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunnel

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"sync/atomic"

	"github.com/lestrrat-go/jwx/v3/jwt"
	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/sshproxy"
)

// RelayHandler implements the JIMM relay upgrade endpoint:
//
//	GET /ssh-relay/:virtualHostname
//
// JIMM authenticates with a bearer JWT in the Authorization header (the
// same external-auth flow the API server already supports on HTTP
// endpoints). The JWT carries the user's SSH public key as a claim
// (ssh_public_key), binding the relayed session to the exact key the user
// presented to JIMM. After the upgrade, the user's SSH session - relayed
// blind by JIMM - terminates in the embedded SSH server built here.
type RelayHandler struct {
	config RelayHandlerConfig

	// concurrentConnections holds the number of concurrent relayed
	// sessions.
	concurrentConnections atomic.Int32
}

// RelayHandlerConfig holds the configuration for the JIMM relay endpoint.
type RelayHandlerConfig struct {
	// Logger is used for logging.
	Logger logger.Logger
	// Authorizer checks whether the user identified by the JWT may access
	// the destination.
	Authorizer RelayAuthorizer
	// ProxyFactory creates target-specific session, forwarding, and SFTP
	// handlers for the terminating SSH server.
	ProxyFactory sshproxy.ProxyFactory
	// SSHService resolves terminating SSH host keys for virtual
	// destinations.
	SSHService sshproxy.SSHService
	// MaxConcurrentConnections is the maximum number of concurrent relayed
	// sessions.
	MaxConcurrentConnections int
	// Metrics collects connection metrics.
	Metrics MetricsCollector
}

// Validate checks whether the configuration is valid.
func (cfg RelayHandlerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.New("nil Logger")
	}
	if cfg.Authorizer == nil {
		return errors.New("nil Authorizer")
	}
	if cfg.ProxyFactory == nil {
		return errors.New("nil ProxyFactory")
	}
	if cfg.SSHService == nil {
		return errors.New("nil SSHService")
	}
	if cfg.MaxConcurrentConnections <= 0 {
		return errors.New("non-positive MaxConcurrentConnections")
	}
	if cfg.Metrics == nil {
		return errors.New("nil Metrics")
	}
	return nil
}

// RelayAuthorizer checks whether the user identified by a JWT may access a
// destination.
type RelayAuthorizer interface {
	// Authorize checks whether the user identified by token may access the
	// target destination.
	Authorize(ctx context.Context, token jwt.Token, destination virtualhostname.Info) (bool, error)
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
	// flow); the token was validated by the HTTP authentication layer.
	token, ok := ctx.Value(RelayJWTKey{}).(jwt.Token)
	if !ok || token == nil {
		http.Error(w, "missing relay JWT", http.StatusUnauthorized)
		return
	}

	// Enforce the user's SSH public key claim: the relayed session is
	// bound to the exact key the user presented to JIMM. The embedded
	// terminating server authenticates the key end to end.
	expectedPublicKey, err := parseSSHPublicKeyClaim(token)
	if err != nil {
		h.config.Logger.Errorf(ctx, "invalid ssh public key claim: %v", err)
		http.Error(w, "invalid ssh public key claim", http.StatusUnauthorized)
		return
	}

	if ok, err := h.config.Authorizer.Authorize(ctx, token, destination); err != nil {
		h.config.Logger.Errorf(ctx, "authorizing relay access: %v", err)
		http.Error(w, "failed to authorize access to destination", http.StatusInternalServerError)
		return
	} else if !ok {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}

	// Enforce the connection limit before upgrading. Rejected requests
	// are not counted as connections.
	current := h.concurrentConnections.Add(1)
	if int(current) > h.config.MaxConcurrentConnections {
		h.concurrentConnections.Add(-1)
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}
	h.config.Metrics.IncConnectionCount()
	defer func() {
		h.concurrentConnections.Add(-1)
		h.config.Metrics.DecConnectionCount()
	}()

	// Build the embedded terminating server and resolve the target host
	// key before upgrading, so failures reach JIMM as HTTP errors.
	handlers, err := h.config.ProxyFactory.New(destination)
	if err != nil {
		h.config.Logger.Errorf(ctx, "creating proxy handlers: %v", err)
		http.Error(w, "failed to create embedded server", http.StatusInternalServerError)
		return
	}
	terminatingHostKey, err := h.config.SSHService.VirtualHostKey(ctx, destination)
	if err != nil {
		h.config.Logger.Errorf(ctx, "resolving host key: %v", err)
		http.Error(w, "failed to resolve host key", http.StatusInternalServerError)
		return
	}
	signer, err := gossh.ParsePrivateKey([]byte(terminatingHostKey))
	if err != nil {
		h.config.Logger.Errorf(ctx, "parsing host key: %v", err)
		http.Error(w, "failed to parse host key", http.StatusInternalServerError)
		return
	}

	conn, err := hijack(w, r, RelayUpgradeToken)
	if err != nil {
		h.config.Logger.Errorf(ctx, "upgrading relay connection: %v", err)
		return
	}
	stop := watchDying(conn, dyingFromContext(ctx), h.config.Logger)
	defer stop()
	defer func() { _ = conn.Close() }()

	// Terminate the relayed user SSH session here. JIMM cannot read the
	// session bytes; the embedded server handles them end to end. The
	// PublicKeyHandler binds the session to the exact key carried in the
	// JWT's ssh_public_key claim, closing the loop between the HTTP-layer
	// claim and the SSH-layer authentication.
	server := sshproxy.NewTerminatingSSHServer(handlers)
	server.PublicKeyHandler = func(_ ssh.Context, key ssh.PublicKey) error {
		if !bytes.Equal(key.Marshal(), expectedPublicKey.Marshal()) {
			return errors.New("public key does not match JWT claim")
		}
		return nil
	}
	server.AddHostKey(signer)
	server.HandleConn(conn)
}

// parseSSHPublicKeyClaim extracts the ssh_public_key claim from the JWT
// and parses it into a gossh.PublicKey. The claim is minted by JIMM
// (jujuauth.NewSSHToken) as the base64-encoded, marshalled user SSH
// public key. The embedded terminating server authenticates the key
// end to end; this parse binds the relayed session to the same key at
// the HTTP layer.
func parseSSHPublicKeyClaim(token jwt.Token) (gossh.PublicKey, error) {
	var encoded string
	if err := token.Get(SSHPublicKeyClaim, &encoded); err != nil {
		return nil, errors.Errorf("missing %s claim", SSHPublicKeyClaim)
	}
	if encoded == "" {
		return nil, errors.Errorf("empty %s claim", SSHPublicKeyClaim)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Errorf("decoding %s claim: %w", SSHPublicKeyClaim, err)
	}
	key, err := gossh.ParsePublicKey(decoded)
	if err != nil {
		return nil, errors.Errorf("parsing %s claim: %w", SSHPublicKeyClaim, err)
	}
	return key, nil
}

// SSHPublicKeyClaim is the JWT claim carrying the base64-encoded user SSH
// public key. It is minted by JIMM (jujuauth.NewSSHToken) and asserted here.
const SSHPublicKeyClaim = "ssh_public_key"

// RelayJWTKey is the context key for the relay JWT, set by the apiserver
// from the request's auth info.
type RelayJWTKey struct{}

// DyingKey is the context key for the apiserver dying signal.
type DyingKey struct{}
