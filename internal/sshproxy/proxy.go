// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	ssh "github.com/tailscale/gliderssh"

	"github.com/juju/juju/core/virtualhostname"
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

// ProxyFactory creates handlers for an SSH target.
type ProxyFactory interface {
	// New validates the destination matches a supported target type
	// and returns a set of handlers for the target.
	New(virtualhostname.Info) (ProxyHandlers, error)
}

// NewTerminatingSSHServer returns an embedded SSH server that terminates an
// SSH connection and proxies it to a routed target using the given handlers.
// Callers may further configure the returned server (for example, adding a
// PublicKeyHandler or host key) before serving a connection.
func NewTerminatingSSHServer(handlers ProxyHandlers) *ssh.Server {
	return &ssh.Server{
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session":      ssh.DefaultSessionHandler,
			"direct-tcpip": handlers.DirectTCPIPHandler(),
		},
		Handler: func(session ssh.Session) {
			handlers.SessionHandler(session)
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": handlers.SFTPHandler(),
		},
	}
}
