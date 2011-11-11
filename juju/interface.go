package juju

import "launchpad.net/juju/go/schema"

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// ConfigChecker is used to check sections of the environments.yaml
	// file that specify this provider. The value passed to the Checker is
	// that returned from the yaml parse, of type schema.MapType.
	ConfigChecker() schema.Checker

	// NewEnviron creates a new Environ with
	// the given attributes returned by the ConfigChecker.
	// The name is that given in environments.yaml.
	Open(name string, attributes interface{}) (Environ, error)
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
	Bootstrap() error

	// FindMachineSpec finds a possible machine specification matching the
	// given constraint, with the goal of minimising cost if all else is equal.
	//	FindMachineSpec(constraint *MachineConstraint) (MachineSpec, error)

	// StartMachine asks for a new machine instance to be created.
	// TODO add arguments to specify type of new machine
	// and zookeeper instances.
	StartMachine(id string) (Machine, error)

	// StopMachine shuts down the given Machine.
	StopMachines([]Machine) error

	// Machines returns the list of currently started instances.
	Machines() ([]Machine, error)

	// Destroy shuts down all known machines and destroys the
	// rest of the environment.
	Destroy() error
}
