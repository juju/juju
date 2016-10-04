// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	manualcommon "github.com/juju/juju/environs/manual/common"
	"github.com/juju/juju/environs/manual/linux"
	"github.com/juju/juju/instance"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.environs.manual")

// Provisioner is the process of provisioning a windows/linux machine
type Provisioner interface {
	// Provision is the main entrypoint of the Provisioner
	// The Make will return an machineID and err if any.
	Provision() (string, error)
}

// ProvisionMachine provisions a machine agent to an existing host, via
// an SSH connection or WinRM connection to the specified host. The host may optionally be preceded
// with a login username, as in [user@]host.
//
// On successful completion, this function will return the id of the state.Machine
// that was entered into state.
func ProvisionMachine(args manualcommon.ProvisionMachineArgs, placement *instance.Placement) (machineId string, err error) {
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			if cleanupErr := args.Client.ForceDestroyMachines(machineId); cleanupErr != nil {
				logger.Errorf("error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
	}()

	var p Provisioner
	switch placement.Scope {
	case linux.Scope:
		p = linux.NewProvisioner(args)
	default:
		return machineId, manualcommon.ErrNoProtoScope
	}

	return p.Provision()
}
