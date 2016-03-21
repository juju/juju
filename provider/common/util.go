// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "fmt"

// EnvFullName returns a string based on the provided model
// UUID that is suitable for identifying the env on a provider.
//
// The resulting string clearly associates the value with juju,
// whereas the model's UUID alone isn't very distinctive for humans.
// This benefits users by helping them quickly identify in their
// hosting management tools which instances are juju related.
func EnvFullName(modelUUID string) string {
	return fmt.Sprintf("juju-%s", modelUUID)
}

// MachineFullName returns a string based on the provided model
// UUID and machine ID that is suitable for identifying instances
// on a provider. See EnvFullName for an explanation on how this
// function helps juju users.
func MachineFullName(modelUUID, machineID string) string {
	modelstr := EnvFullName(modelUUID)
	return fmt.Sprintf("%s-machine-%s", modelstr, machineID)
}
