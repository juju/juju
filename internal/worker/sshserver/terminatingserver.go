// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/juju/errors"
	gliderssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
)

// TerminatingServerFactory builds terminating SSH servers for routed
// destinations. It is the type the worker's manifold outputs.
type TerminatingServerFactory struct {
	factory ProxyFactory
	svc     SSHService
}

// newTerminatingServerFactory returns a TerminatingServerFactory.
func newTerminatingServerFactory(factory ProxyFactory, svc SSHService) *TerminatingServerFactory {
	return &TerminatingServerFactory{factory: factory, svc: svc}
}

// New returns a terminating SSH server for the destination, with proxy
// handlers and the destination's host key configured.
func (f *TerminatingServerFactory) New(ctx context.Context, destination virtualhostname.Info) (*gliderssh.Server, error) {
	handlers, err := f.factory.New(destination)
	if err != nil {
		return nil, errors.Trace(err)
	}
	key, err := f.svc.VirtualHostKey(ctx, destination)
	if err != nil {
		return nil, errors.Trace(err)
	}
	signer, err := gossh.ParsePrivateKey([]byte(key))
	if err != nil {
		return nil, errors.Trace(err)
	}
	server := newTerminatingSSHServer(handlers)
	server.AddHostKey(signer)
	return server, nil
}
