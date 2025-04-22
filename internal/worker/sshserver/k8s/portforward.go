// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// DirectTCPIPHandler returns a handler for the DirectTCPIP channel type.
// It immediately rejects the channel request since we don't currently
// support port forwarding for K8s units.
func (Handlers) DirectTCPIPHandler() ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		_ = newChan.Reject(gossh.Prohibited, "not implemented")
	}
}
