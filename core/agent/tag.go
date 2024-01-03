// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import "github.com/juju/names/v5"

// IsAllowedControllerTag returns true if the tag kind can be for a controller.
// TODO(controlleragent) - this method is needed while IAAS controllers are still machines.
func IsAllowedControllerTag(kind string) bool {
	return kind == names.ControllerAgentTagKind || kind == names.MachineTagKind
}
