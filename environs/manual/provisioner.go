// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/environs/manual/common"
	"github.com/juju/juju/environs/manual/linux"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.environs.manual")

// Provisioner is the process of provisioning a windows/linux machine
type Provisioner interface {
	// Make is the main entrypoint of the Provisioner
	// The Make will return an machineID and err if any.
	Make() (string, error)
}

// ProvisionMachine provisions a machine agent to an existing host, via
// an SSH connection to the specified host. The host may optionally be preceded
// with a login username, as in [user@]host.
//
// On successful completion, this function will return the id of the state.Machine
// that was entered into state.
func ProvisionMachine(args common.ProvisionMachineArgs, scope string) (machineId string, err error) {
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			if cleanupErr := args.Client.ForceDestroyMachines(machineId); cleanupErr != nil {
				logger.Errorf("error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
	}()

	var provision Provisioner
	switch scope {
	case linux.Scope:
		provision = linux.NewProvision(args)
	}

	if provision == nil {
		return machineId, common.ErrNoProtoScope
	}

	return provision.Make()
}
