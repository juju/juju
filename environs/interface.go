package environs

import (
	"errors"
	"io"
	"launchpad.net/juju/go/state"
)

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// Check is used to validate the configuration attributes.
	// The attributes returned on successful validation are 
	// specific to the Provider.
	Check(attributes interface{}) (attributes interface{}, err error)

	// Open creates a new Environ with the attributes returned by Check. 
	// The name is that given in environments.yaml.
	Open(name string, attributes interface{}) (Environ, error)
}

var ErrNoDNSName = errors.New("DNS name not allocated")

// Instance represents the provider-specific notion of a machine.
type Instance interface {
	// Id returns a provider-generated identifier for the Instance.
	Id() string

	// DNSName returns the DNS name for the instance.
	// If the name is not yet allocated, it will return
	// an ErrNoDNSName error.
	DNSName() (string, error)

	// WaitDNSName returns the DNS name for the instance,
	// waiting until it is allocated if necessary.
	WaitDNSName() (string, error)
}

var ErrNoInstances = errors.New("no instances found")
var ErrPartialInstances = errors.New("only some instances were found")

// An Environ represents a juju environment as specified
// in the environments.yaml file.
// 
// Due to the limitations of some providers (for example ec2), the
// results of the Environ methods may not be fully sequentially
// consistent. In particular, while a provider may retry when it
// gets an error for an operation, it will not retry when
// an operation succeeds, even if that success is not
// consistent with a previous operation.
// 
type Environ interface {
	// Bootstrap initializes the state for the environment,
	// possibly starting one or more instances.
	Bootstrap() error

	// StateInfo returns information on the state initialized
	// by Bootstrap.
	StateInfo() (*state.Info, error)

	// StartInstance asks for a new instance to be created,
	// associated with the provided machine identifier.
	// The given info describes the juju state for the new
	// instance to connect to.
	// TODO add arguments to specify type of new machine.
	StartInstance(machineId int, info *state.Info) (Instance, error)

	// StopInstances shuts down the given instances.
	StopInstances([]Instance) error

	// Instances returns a slice of instances corresponding to the
	// given instance ids.  If no instances were found, but there
	// was no other error, it will return ErrNoInstances.  If
	// some but not all the instances were found, the returned slice
	// will have some nil slots, and an ErrPartialInstances error
	// will be returned.
	Instances(ids []string) ([]Instance, error)

	// PutFile reads from r and writes to the given file in the
	// environment's storage. The length must give the total
	// length of the file.
	//
	// If the name is prefixed with "tools/", it should be an
	// archive of the juju tools in gzipped tar format; the full
	// name should be of the form:
	//     tools/juju-$VERSION-$GOOS-$GOARCH.tgz
	PutFile(file string, r io.Reader, length int64) error

	// GetFile opens the given file in the environment's storage
	// and returns a ReadCloser that can be used to read its
	// contents. It is the caller's responsibility to close it
	// after use.
	GetFile(file string) (io.ReadCloser, error)

	// RemoveFile removes the given file from the environment's storage.
	// It is not an error to remove a file that does not exist.
	RemoveFile(file string) error

	// Destroy shuts down all known machines and destroys the
	// rest of the environment. A list of instances known to
	// be part of the environment can be given with insts.
	// This is because recently started machines might not
	// yet be visible in the environment, so this method
	// can wait until they are.
	Destroy(insts []Instance) error
}
