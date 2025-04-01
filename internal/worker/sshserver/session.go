// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/gliderlabs/ssh"
)

type stubSessionHandler struct{}

// Handle is a stub implementation of the SessionHandler interface.
// It currently does nothing but will be used to proxy a user's SSH session
// to a target machine.
func (s *stubSessionHandler) Handle(session ssh.Session, destinationAddress string) {
}
