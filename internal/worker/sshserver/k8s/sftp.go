// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"github.com/gliderlabs/ssh"
)

// SFTPHandler returns a handler for the SFTP subsystem.
// It is not currently implemented for K8s models.
func (s *Handlers) SFTPHandler() ssh.SubsystemHandler {
	return func(session ssh.Session) {
		_, _ = session.Stderr().Write([]byte("not implemented" + "\n"))
		_ = session.Exit(1)
	}
}
