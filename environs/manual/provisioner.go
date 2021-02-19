// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"io"

	"github.com/juju/utils/v2/winrm"

	"github.com/juju/juju/apiserver/params"
)

var (
	// ErrProvisioned is returned by ProvisionMachine if the target
	// machine has an existing machine agent.
	ErrProvisioned = errors.New("machine is already provisioned")
)

// ProvisionMachineFunc that every provisioner should have
type ProvisionMachineFunc func(ProvisionMachineArgs) (machineId string, err error)

// ProvisionMachineArgs used for arguments for the Provisioner methods
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

	// WinRM contains keys and client interface api with the remote windows machine
	WinRM WinRMArgs

	*params.UpdateBehavior
}

// WinRMArgs used for providing special context
// on how we interface with the windows machine
type WinRMArgs struct {
	// Keys that contains CACert, ClientCert, ClientKey
	Keys *winrm.X509

	// Client for interacting with windows machines
	Client WinrmClientAPI
}

// WinrmClientAPI minimal interface for winrm windows machines interactions
type WinrmClientAPI interface {
	Ping() error
	Run(cmd string, stdout, stderr io.Writer) error
	Password() string
}

// ProvisioningClientAPI defines the methods that are needed for the manual
// provisioning of machines.  An interface is used here to decouple the API
// consumer from the actual API implementation type.
type ProvisioningClientAPI interface {
	AddMachines([]params.AddMachineParams) ([]params.AddMachinesResult, error)
	ForceDestroyMachines(machines ...string) error
	ProvisioningScript(params.ProvisioningScriptParams) (script string, err error)
}
