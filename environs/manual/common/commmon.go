package common

import (
	"errors"
	"fmt"
	"io"

	"github.com/juju/juju/apiserver/params"
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
	// Host is the SSH host: [user@]host
	Host string
	// User is the SSH/WINRM user [user@]host
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
