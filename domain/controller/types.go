// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

const (
	// ControllerSSHKeyComment is the comment used in ssh public key files to
	// represent the public key owned by a controller.
	ControllerSSHKeyComment = "juju-system-key"
)

// ControllerInfo contains information about the current controller.
type ControllerInfo struct {
	UUID         string
	CACert       string
	APIAddresses []string
}
