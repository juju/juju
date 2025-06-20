// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import "github.com/juju/juju/core/machine"

// CreateMachineArgs contains arguments for creating a machine.
type CreateMachineArgs struct {
	// Parent is the name of the parent machine.
	Parent machine.Name

	// LXDProfiles is the list of LXD profiles to apply to the machine.
	LXDProfiles []string

	// InstanceType is the instance type to use for the machine.
	InstanceType string

	// InstanceID is the instance ID to use for the machine.
	InstanceID string

	// InstanceTags is the list of instance tags to apply to the machine.
	InstanceTags []string

	// Nonce is the nonce to use for the machine.
	Nonce *string
}
