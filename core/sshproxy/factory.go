// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"

	ssh "github.com/tailscale/gliderssh"

	"github.com/juju/juju/core/virtualhostname"
)

// TerminatingServerFactory builds terminating SSH servers for routed
// destinations. It is satisfied by the SSH server worker's factory, whose
// output the apiserver manifold consumes for the relay endpoint.
type TerminatingServerFactory interface {
	// New returns a terminating SSH server for the destination, with proxy
	// handlers and the destination's host key configured.
	New(ctx context.Context, destination virtualhostname.Info) (*ssh.Server, error)
}
