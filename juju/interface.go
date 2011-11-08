package juju

import (
	"os"
	"launchpad.net/juju/go/schema"
)

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// ConfigChecker is used to check sections of the environments.yaml
	// file that specify this provider. The value passed to the Checker is
	// that returned from the yaml parse, of type schema.MapType.
	ConfigChecker() schema.Checker

	// NewEnviron creates a new Environ with
	// the given attributes returned by the ConfigChecker.
	// The name is that given in environments.yaml.
	NewEnviron(name string, attributes interface{}) (Environ, os.Error)
}

// MachineConstraint specifies a range of possible machine
// types, including the OS that's running on the machine.
// TODO change to specify softer constraints and so that it's not EC2 specific.
// TODO change to specify more than just the OS image.
type MachineConstraint struct {
	UbuntuRelease     string
	Architecture      string
	PersistentStorage bool
	Region            string
	Daily             bool
	Desktop           bool
}

// This may move into the Environ interface.
var DefaultMachineConstraint = &MachineConstraint{
	UbuntuRelease:     "oneiric",
	Architecture:      "i386",
	PersistentStorage: true,
	Region:            "us-east-1",
	Daily:             false,
	Desktop:           false,
}

// MachineSpec represents one possible machine configuration
// obtainable from a provider.
// TODO change to specify more than just the OS.
type MachineSpec interface {
	UbuntuRelease() string
}

// Machine represents a running machine instance.
type Machine interface {
	Id() string
	DNSName() string
}

// An Environ represents a juju environment as specified
// in the environments.yaml file.
type Environ interface {
	// Bootstrap initializes the new environment.
	Bootstrap() os.Error

	// FindMachineSpec finds a possible machine specification matching the
	// given constraint, with the goal of minimising cost if all else is equal.
	FindMachineSpec(constraint *MachineConstraint) (MachineSpec, os.Error)

	// StartMachine asks for a new machine instance to be created.
	// The machine is identified with machineId, and spec gives the
	// machine's requested specification. 
	// The spec must have been created with the same Environ's
	// FindMachineSpec method
	// 
	// TODO add arguments to specify zookeeper instances.
	StartMachine(machineId string, spec MachineSpec) (Machine, os.Error)

	// StopMachine shuts down the given Machine.
	StopMachines([]Machine) os.Error

	// Machines returns the list of currently started instances.
	Machines() ([]Machine, os.Error)

	// Destroy shuts down all known machines and destroys the
	// rest of the environment.
	Destroy() os.Error
}
