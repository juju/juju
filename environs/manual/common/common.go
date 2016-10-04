// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"

	"github.com/juju/juju/apiserver/params"

	"github.com/juju/errors"
)

// ProvisioningClientAPI defines the methods that are needed for the manual
// provisioning of machines.  An interface is used here to decouple the API
// consumer from the actual API implementation type.
type ProvisioningClientAPI interface {
	AddMachines([]params.AddMachineParams) ([]params.AddMachinesResult, error)
	ForceDestroyMachines(machines ...string) error
	ProvisioningScript(params.ProvisioningScriptParams) (script string, err error)
}

var (
	// ErrProvisioned is returned by ProvisionMachine if the target
	// machine has an existing machine agent.
	ErrProvisioned = errors.New("machine is already provisioned")
	// ErrNoProtoScope is returned on ProvisionMachine when we are not in the ssh
	// or winrm scope and we must continue the provisioning process
	ErrNoProtoScope = fmt.Errorf("No winrm/ssh scope")
)

const ManualInstancePrefix = "manual:"

type ProvisionMachineArgs struct {
	// user and host of the ssh or winrm conn
	Host string
	User string

	// DataDir is the root directory for juju data.
	// If left blank, the default location "/var/lib/juju" will be used.
	DataDir string

	// Client provides the API needed to provision the machines.
	Client ProvisioningClientAPI

	// Stdin is required to respond to sudo prompts,
	// and must be a terminal (except in tests)
	Stdin io.Reader

	// Stdout is required to present sudo prompts to the user.
	Stdout io.Writer

	// Stderr is required to present machine provisioning progress to the user.
	Stderr io.Writer

	// AuthorizedKeys contains the concatenated authorized-keys to add to the
	// ubuntu user's ~/.ssh/authorized_keys.
	AuthorizedKeys string

	*params.UpdateBehavior
}

// ScriptProvisioner used to determine the script that will be used to
// manual provision a machine linux or windows machine.
type ScriptProvisioner interface {
	ProvisioningScript() (string, error)
}

// RecordMachineInstate records and saves into the state machine the provisioned machine
func RecordMachineInState(client ProvisioningClientAPI, machineParams params.AddMachineParams) (machineId string, err error) {
	results, err := client.AddMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return "", err
	}
	// Currently, only one machine is added, but in future there may be several added in one call.
	machineInfo := results[0]
	if machineInfo.Error != nil {
		return "", machineInfo.Error
	}
	return machineInfo.Machine, nil
}
