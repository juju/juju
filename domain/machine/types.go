// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import "github.com/juju/juju/core/machine"

const ManualInstancePrefix = "manual:"

// ExportMachine represents a machine that is being exported to another
// controller.
type ExportMachine struct {
	Name         machine.Name
	UUID         machine.UUID
	Nonce        string
	PasswordHash string
}
