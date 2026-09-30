// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/juju/errors"
	gliderssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	coresshproxy "github.com/juju/juju/core/sshproxy"
	"github.com/juju/juju/core/virtualhostname"
)

// newTerminatingServerFactory returns a TerminatingServerFactory.
func newTerminatingServerFactory(factory ProxyFactory, svc SSHService) coresshproxy.TerminatingServerFactory {
	return terminatingServerFactory{factory: factory, svc: svc}
}

type terminatingServerFactory struct {
	factory ProxyFactory
	svc     SSHService
}

// New implements TerminatingServerFactory.
func (f terminatingServerFactory) New(ctx context.Context, destination virtualhostname.Info) (*gliderssh.Server, error) {
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
