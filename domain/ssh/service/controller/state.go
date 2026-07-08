// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import "context"

// State describes controller-scoped persistence for SSH host keys.
type State interface {
	// GetSSHServerHostKey returns the stored controller jump host key.
	GetSSHServerHostKey(context.Context) (string, error)
}
