// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/juju/errors"
	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
)

// TerminatingServerFactory builds terminating SSH servers for routed
// destinations.
type TerminatingServerFactory interface {
	// New returns a terminating SSH server for the destination, with proxy
	// handlers and the destination's host key configured.
	New(ctx context.Context, destination virtualhostname.Info) (*ssh.Server, error)
}

// NewTerminatingServerFactory returns a TerminatingServerFactory.
func NewTerminatingServerFactory(factory ProxyFactory, svc SSHService) TerminatingServerFactory {
	return terminatingServerFactory{factory: factory, svc: svc}
}

type terminatingServerFactory struct {
	factory ProxyFactory
	svc     SSHService
}

// New implements TerminatingServerFactory.
func (r terminatingServerFactory) New(ctx context.Context, destination virtualhostname.Info) (*ssh.Server, error) {
	handlers, err := r.factory.New(destination)
	if err != nil {
		return nil, errors.Trace(err)
	}
	key, err := r.svc.VirtualHostKey(ctx, destination)
	if err != nil {
		return nil, errors.Trace(err)
	}
	signer, err := gossh.ParsePrivateKey([]byte(key))
	if err != nil {
		return nil, errors.Trace(err)
	}
	server := NewTerminatingSSHServer(handlers)
	server.AddHostKey(signer)
	return server, nil
}
