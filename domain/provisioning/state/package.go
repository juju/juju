// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/domain/provisioning/service"
)

// Compile-time interface assertions.
var (
	_ service.ModelState      = (*ModelState)(nil)
	_ service.ControllerState = (*ControllerState)(nil)
)
