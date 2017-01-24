// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"net"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

var netLookupHost = net.LookupHost

const ManualInstancePrefix = "manual:"

// RecordMachineInState records and saves into the state machine the provisioned machine
func RecordMachineInState(client ProvisioningClientAPI, machineParams params.AddMachineParams) (machineId string, err error) {
	results, err := client.AddMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return "", errors.Trace(err)
	}
	// Currently, only one machine is added, but in future there may be several added in one call.
	machineInfo := results[0]
	if machineInfo.Error != nil {
		return "", machineInfo.Error
	}
	return machineInfo.Machine, nil
}
