// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/worker/sshserver/handlers/machine"
)

// ProxyHandlers provide session, local forwarding, and SFTP handling for a target.
type ProxyHandlers interface {
	// SessionHandler returns a handler for proxying SSH commands/terminal sessions.
	SessionHandler(ssh.Session)
	// DirectTCPIPHandler returns a handler for proxying SSH local forwarding requests.
	DirectTCPIPHandler() ssh.ChannelHandler
	// SFTPHandler returns a handler for proxying SFTP requests.
	SFTPHandler() ssh.SubsystemHandler
}

type proxyFactory struct {
	logger    logger.Logger
	connector machine.SSHConnector
}

// New returns a set of handlers for the given target based
// on whether the target is a container, unit or machine.
func (f proxyFactory) New(destination virtualhostname.Info) (ProxyHandlers, error) {
	switch destination.Target() {
	case virtualhostname.ContainerTarget:
		return nil, errors.NotImplemented
	case virtualhostname.MachineTarget, virtualhostname.UnitTarget:
		return machine.NewHandlers(destination, f.connector, f.logger)
	default:
		return nil, errors.NotValidf("unknown virtual hostname target %d", destination.Target())
	}
}
