// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"

	"github.com/juju/juju/controller"
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

// ControllerConfigService is the interface for reading controller
// configuration, used to determine the controller SSH jump server port.
type ControllerConfigService interface {
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(ctx context.Context) (controller.Config, error)
}

// ControllerSSHHostKeyService is the interface for reading the controller SSH
// jump server host key.
type ControllerSSHHostKeyService interface {
	// SSHServerHostKey returns the controller SSH jump server host key. Note
	// this is the private key material; the facade derives the public key from
	// it before returning it to agents.
	SSHServerHostKey(ctx context.Context) (string, error)
}
