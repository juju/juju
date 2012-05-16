package environs

import (
	"errors"
	"io"
	"launchpad.net/juju/go/schema"
	"launchpad.net/juju/go/state"
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

type NotFoundError struct {
	// error gives the underlying error.
	error
}

// StorageReader gives a way to read data stored within
// an environment. When a method is called on a
// non-existent file, the concrete type of the returned error
// should be *NotFoundError.
type StorageReader interface {
	// Get opens the given storage file
	// and returns a ReadCloser that can be used to read its
	// contents. It is the caller's responsibility to close it
	// after use.
	Get(name string) (io.ReadCloser, error)

	// List lists all file names in the storage with the given prefix,
	// in alphabetical order.
	List(prefix string) ([]string, error)

	// TODO: URL returns a URL that can be used to access the given
	// storage file.
	// URL(name string) (string, error)
}

// StorageWriter gives a way to change data stored
// within an environment.
type StorageWriter interface {
	// Put reads from r and writes to the given storage file.
	// The length must give the total length of the file.
	Put(name string, r io.Reader, length int64) error

	// Remove removes the given file from the environment's
	// storage. It should not return an error if the file does
	// not exist.
	Remove(name string) error
}

// Storage represents storage that can be both
// read and written.
type Storage interface {
	StorageReader
	StorageWriter
}

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
	// If uploadTools is true, the current version of
	// the juju tools will be uploaded and used
	// on the environment's instances.
	Bootstrap(uploadTools bool) error

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

	// Storage returns storage specific to the environment.
	Storage() Storage

	// PublicStorage returns storage shared between environments.
	PublicStorage() StorageReader

	// Destroy shuts down all known machines and destroys the
	// rest of the environment. A list of instances known to
	// be part of the environment can be given with insts.
	// This is because recently started machines might not
	// yet be visible in the environment, so this method
	// can wait until they are.
	Destroy(insts []Instance) error
}
