// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"context"
	"net"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

var netLookupHost = net.LookupHost

// RecordMachineInState records and saves into the state machine the provisioned machine
func RecordMachineInState(ctx context.Context, client ProvisioningClientAPI, machineParams params.AddMachineParams) (machineId string, err error) {
	results, err := client.AddMachines(ctx, []params.AddMachineParams{machineParams})
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
