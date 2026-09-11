// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"

	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
)

// Termination carries the pieces needed to terminate an SSH session for a
// routed destination, with the destination-specific host key material
// already resolved.
type Termination struct {
	// Handlers are the per-destination session, forwarding, and SFTP
	// handlers.
	Handlers ProxyHandlers
	// Signer is the terminating host key signer for the destination.
	Signer gossh.Signer
}

// Resolver constructs per-destination terminations by bundling the proxy
// handler factory with the SSH service that resolves terminating host keys.
// Both the jump-server direct-tcpip path and the apiserver relay endpoint
// consume it through one seam.
type Resolver interface {
	// Resolve validates the destination, builds the proxy handlers, and
	// resolves the destination's terminating host key.
	Resolve(ctx context.Context, destination virtualhostname.Info) (Termination, error)
}

// NewResolver returns a Resolver bundling the given factory and service.
// Callers obtain a per-destination termination with one call rather than
// sequencing factory and host key lookups themselves.
func NewResolver(factory ProxyFactory, svc SSHService) Resolver {
	return resolver{factory: factory, svc: svc}
}

type resolver struct {
	factory ProxyFactory
	svc     SSHService
}

// Resolve builds the proxy handlers and resolves the destination's
// terminating host key.
func (r resolver) Resolve(ctx context.Context, destination virtualhostname.Info) (Termination, error) {
	handlers, err := r.factory.New(destination)
	if err != nil {
		return Termination{}, errors.Trace(err)
	}
	key, err := r.svc.VirtualHostKey(ctx, destination)
	if err != nil {
		return Termination{}, errors.Trace(err)
	}
	signer, err := gossh.ParsePrivateKey([]byte(key))
	if err != nil {
		return Termination{}, errors.Trace(err)
	}
	return Termination{Handlers: handlers, Signer: signer}, nil
}
