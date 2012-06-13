package environs

import (
	"errors"
	"io"
	"launchpad.net/juju-core/juju/state"
)

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// NewConfig returns a new EnvironConfig representing the
	// environment with the given attributes.  Every provider must
	// accept the "name" and "type" keys, holding the name of the
	// environment and the provider type respectively.
	NewConfig(attrs map[string]interface{}) (EnvironConfig, error)
}

// EnvironConfig represents an environment's configuration.
type EnvironConfig interface {
	// Open opens the environment and returns it.
	Open() (Environ, error)
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

// NotFoundError records an error when something has not been found.
type NotFoundError struct {
	// error is the underlying error.
	error
}

// A StorageReader can retrieve and list files from a storage provider.
type StorageReader interface {
	// Get opens the given storage file and returns a ReadCloser
	// that can be used to read its contents.  It is the caller's
	// responsibility to close it after use.  If the name does not
	// exist, it should return a *NotFoundError.
	Get(name string) (io.ReadCloser, error)

	// List lists all names in the storage with the given prefix, in
	// alphabetical order.  The names in the storage are considered
	// to be in a flat namespace, so the prefix may include slashes
	// and the names returned are the full names for the matching
	// entries.
	List(prefix string) ([]string, error)

	// URL returns a URL that can be used to access the given storage file.
	URL(name string) (string, error)
}

// A StorageWriter adds and removes files in a storage provider.
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

	// SetConfig updates the Environs configuration.
	// Calls to SetConfig do not affect the configuration of
	// values previously obtained from Storage and PublicStorage.
	SetConfig(config EnvironConfig)

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
	// If an empty sized list of ids is passed, all the currently 
	// known Instances will be returned.
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
	//
	// When Destroy has been called, any Environ referring to the
	// same remote environment may become invalid
	Destroy(insts []Instance) error

	// AssignmentPolicy returns the environment's unit assignment policy.
	AssignmentPolicy() state.AssignmentPolicy
}
