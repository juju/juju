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

	// StartMachine asks for a new machine instance to be created.
	// TODO add arguments to specify type of new machine
	// and zookeeper instances.
	StartMachine() (Machine, os.Error)

	// StopMachine shuts down the given Machine.
	StopMachines([]Machine) os.Error

	// Machines returns the list of currently started instances.
	Machines() ([]Machine, os.Error)

	// Destroy shuts down all known machines and destroys the
	// rest of the environment.
	Destroy() os.Error
}
