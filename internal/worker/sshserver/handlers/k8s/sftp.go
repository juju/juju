// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import ssh "github.com/tailscale/gliderssh"

// SFTPHandler rejects SFTP, which is unsupported for Kubernetes targets.
func (*Handlers) SFTPHandler() ssh.SubsystemHandler {
	return func(session ssh.Session) {
		_, _ = session.Stderr().Write([]byte("SFTP is not supported for Kubernetes targets\n"))
		_ = session.Exit(1)
	}
}
