// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/environs"
)

// EnvFullName returns a string based on the provided environment
// that is suitable for identifying the env on a provider.
func EnvFullName(env environs.Environ) string {
	envUUID, _ := env.Config().UUID() // Env should have validated this.
	return fmt.Sprintf("juju-%s", envUUID)
}

// MachineFullName returns a string based on the provided environment
// and machine ID that is suitable for identifying instances on a
// provider.
func MachineFullName(env environs.Environ, machineId string) string {
	envstr := EnvFullName(env)
	machineTag := names.NewMachineTag(machineId)
	return fmt.Sprintf("%s-%s", envstr, machineTag)
}
