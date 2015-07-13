// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/juju/environs"
)

// EnvFullName returns a string based on the provided environment
// that is suitable for identifying the env on a provider. The resulting
// string clearly associates the value with juju, whereas the
// environment's UUID alone isn't very distinctive for humans. This
// benefits users by helping them quickly identify in their hosting
// management tools which instances are juju related.
func EnvFullName(env environs.Environ) string {
	envUUID, _ := env.Config().UUID() // Env should have validated this.
	return fmt.Sprintf("juju-%s", envUUID)
}

// MachineFullName returns a string based on the provided environment
// and machine ID that is suitable for identifying instances on a
// provider. See EnvFullName for an explanation on how this function
// helps juju users.
func MachineFullName(env environs.Environ, machineID string) string {
	envstr := EnvFullName(env)
	return fmt.Sprintf("%s-machine-%s", envstr, machineID)
}
