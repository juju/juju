// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/environs"
)

// MachineFullName returns a string base on the provided environment
// and machine ID that is suitable for identifying instances on a
// provider.
func MachineFullName(env environs.Environ, machineId string) string {
	envName := env.Config().Name()
	machineTag := names.NewMachineTag(machineId)
	return fmt.Sprintf("juju-%s-%s", envName, machineTag)
}
