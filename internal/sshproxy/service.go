// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/virtualhostname"
)

// SSHService resolves controller host keys, user public keys, and terminating
// host keys for routed destinations.
type SSHService interface {
	// VirtualHostKey returns the terminating host key for a routed destination.
	VirtualHostKey(context.Context, virtualhostname.Info) (string, error)
	// ResolveK8sExecInfo resolves Kubernetes execution information for a routed
	// destination.
	ResolveK8sExecInfo(context.Context, virtualhostname.Info) (namespace, podName string, err error)
	// MachineForDestination resolves the machine for a routed destination.
	MachineForDestination(context.Context, virtualhostname.Info) (coremachine.Name, error)
	// SSHServerHostKey returns the controller's SSH server host key.
	SSHServerHostKey(context.Context) (string, error)
}
