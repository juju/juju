// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	domainssh "github.com/juju/juju/domain/ssh"
)

// SSHConnRequestService is the interface for watching and reading SSH
// connection requests for the model.
type SSHConnRequestService interface {
	// WatchSSHConnRequest returns a watcher that emits the tunnel IDs of SSH
	// connection requests targeting the named machine.
	WatchSSHConnRequest(ctx context.Context, machineName coremachine.Name) (watcher.StringsWatcher, error)

	// GetSSHConnRequest returns the SSH connection request for the supplied
	// tunnel ID, scoped to the named machine. A request targeting another
	// machine is reported as not found, so a machine agent can only read its
	// own requests.
	GetSSHConnRequest(ctx context.Context, machineName coremachine.Name, tunnelID string) (domainssh.SSHConnRequest, error)
}

// ControllerSSHService is the interface for reading the controller SSH jump
// server host key and listening port. Both come from the SSH domain, which is
// the runtime source of truth: the controller charm pushes the port there, so
// it must not be read from controller config (which may be stale).
type ControllerSSHService interface {
	// SSHServerHostPublicKey returns the marshalled public host key of the
	// controller SSH jump server. The public key is derived once at bootstrap
	// and stored in state, so the facade never handles private key material.
	SSHServerHostPublicKey(ctx context.Context) ([]byte, error)
	// GetSSHServerPort returns the port the controller SSH jump server listens
	// on. The port is owned by the controller charm and pushed to the SSH
	// domain at runtime.
	GetSSHServerPort(ctx context.Context) (int, error)
}
