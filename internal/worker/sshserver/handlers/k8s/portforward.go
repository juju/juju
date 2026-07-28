// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// DirectTCPIPHandler rejects local forwarding, which is unsupported for
// Kubernetes container targets.
func (*Handlers) DirectTCPIPHandler() ssh.ChannelHandler {
	return func(_ *ssh.Server, _ *gossh.ServerConn, newChan gossh.NewChannel, _ ssh.Context) {
		_ = newChan.Reject(gossh.UnknownChannelType, "local forwarding is not supported for Kubernetes targets")
	}
}
