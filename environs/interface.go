package environs

import (
	"errors"
	"io"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// Open opens the environment and returns it.
	Open(cfg *config.Config) (Environ, error)

	// Validate ensures that config is a valid configuration for this
	// provider, applying changes to it if necessary, and returns the
	// validated configuration.
	// If old is not nil, it holds the previous environment configuration
	// for consideration when validating changes.
	Validate(cfg, old *config.Config) (valid *config.Config, err error)

	// SecretAttrs filters the supplied configuation returning only values
	// which are considered sensitive.
	SecretAttrs(cfg *config.Config) (map[string]interface{}, error)
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

	// OpenPorts opens the given ports on the instance, which
	// should have been started with the given machine id.
	OpenPorts(machineId int, ports []state.Port) error

	// ClosePorts closes the given ports on the instance, which
	// should have been started with the given machine id.
	ClosePorts(machineId int, ports []state.Port) error

	// Ports returns the set of ports open on the instance, which
	// should have been started with the given machine id.
	// The ports are returned as sorted by state.SortPorts.
	Ports(machineId int) ([]state.Port, error)
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
	// Name returns the Environ's name.
	Name() string

	// Bootstrap initializes the state for the environment,
	// possibly starting one or more instances.
	// If uploadTools is true, the current version of
	// the juju tools will be uploaded and used
	// on the environment's instances.
	Bootstrap(uploadTools bool) error

	// StateInfo returns information on the state initialized
	// by Bootstrap.
	StateInfo() (*state.Info, error)

	// Config returns the current configuration of this Environ.
	Config() *config.Config

	// SetConfig updates the Environs configuration.
	// Calls to SetConfig do not affect the configuration of
	// values previously obtained from Storage and PublicStorage.
	SetConfig(cfg *config.Config) error

	// StartInstance asks for a new instance to be created,
	// associated with the provided machine identifier.  The given
	// info describes the juju state for the new instance to connect
	// to.  Using the same machine id as another running instance
	// can lead to undefined results. The toolset specifies the
	// juju tools that will run on the new machine - if it is nil,
	// the Environ will find a set of tools compatible with the
	// current version.
	// TODO add arguments to specify type of new machine.
	StartInstance(machineId int, info *state.Info, tools *Toolset) (Instance, error)

	// StopInstances shuts down the given instances.
	StopInstances([]Instance) error

	// Instances returns a slice of instances corresponding to the
	// given instance ids.  If no instances were found, but there
	// was no other error, it will return ErrNoInstances.  If
	// some but not all the instances were found, the returned slice
	// will have some nil slots, and an ErrPartialInstances error
	// will be returned.
	Instances(ids []string) ([]Instance, error)

	// AllInstances returns all instances currently known to the 
	// environment.
	AllInstances() ([]Instance, error)

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

	// Provider returns the EnvironProvider that created this Environ.
	Provider() EnvironProvider
}
