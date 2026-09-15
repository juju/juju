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
