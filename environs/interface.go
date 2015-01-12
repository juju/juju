// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"io"
	"os"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// Prepare prepares an environment for use. Any additional
	// configuration attributes in the returned environment should
	// be saved to be used later. If the environment is already
	// prepared, this call is equivalent to Open.
	Prepare(ctx BootstrapContext, cfg *config.Config) (Environ, error)

	// Open opens the environment and returns it.
	// The configuration must have come from a previously
	// prepared environment.
	Open(cfg *config.Config) (Environ, error)

	// Validate ensures that config is a valid configuration for this
	// provider, applying changes to it if necessary, and returns the
	// validated configuration.
	// If old is not nil, it holds the previous environment configuration
	// for consideration when validating changes.
	Validate(cfg, old *config.Config) (valid *config.Config, err error)

	// Boilerplate returns a default configuration for the environment in yaml format.
	// The text should be a key followed by some number of attributes:
	//    `environName:
	//        type: environTypeName
	//        attr1: val1
	//    `
	// The text is used as a template (see the template package) with one extra template
	// function available, rand, which expands to a random hexadecimal string when invoked.
	BoilerplateConfig() string

	// SecretAttrs filters the supplied configuration returning only values
	// which are considered sensitive. All of the values of these secret
	// attributes need to be strings.
	SecretAttrs(cfg *config.Config) (map[string]string, error)
}

// EnvironStorage implements storage access for an environment.
type EnvironStorage interface {
	// Storage returns storage specific to the environment.
	Storage() storage.Storage
}

// ConfigGetter implements access to an environment's configuration.
type ConfigGetter interface {
	// Config returns the configuration data with which the Environ was created.
	// Note that this is not necessarily current; the canonical location
	// for the configuration data is stored in the state.
	Config() *config.Config
}

// BootstrapParams holds the parameters for bootstrapping an environment.
type BootstrapParams struct {
	// Constraints are used to choose the initial instance specification,
	// and will be stored in the new environment's state.
	Constraints constraints.Value

	// Placement, if non-empty, holds an environment-specific placement
	// directive used to choose the initial instance.
	Placement string

	// AvailableTools is a collection of tools which the Bootstrap method
	// may use to decide which architecture/series to instantiate.
	AvailableTools tools.List

	// ContainerBridgeName, if non-empty, overrides the default
	// network bridge device to use for LXC and KVM containers. See
	// environs.DefaultBridgeName.
	ContainerBridgeName string
}

// BootstrapFinalizer is a function returned from Environ.Bootstrap.
// The caller must pass a MachineConfig with the Tools field set.
type BootstrapFinalizer func(BootstrapContext, *cloudinit.MachineConfig) error

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
// Even though Juju takes care not to share an Environ between concurrent
// workers, it does allow concurrent method calls into the provider
// implementation.  The typical provider implementation needs locking to
// avoid undefined behaviour when the configuration changes.
type Environ interface {
	// Bootstrap creates a new instance with the series and architecture
	// of its choice, constrained to those of the available tools, and
	// returns the instance's architecture, series, and a function that
	// must be called to finalize the bootstrap process by transferring
	// the tools and installing the initial Juju state server.
	//
	// It is possible to direct Bootstrap to use a specific architecture
	// (or fail if it cannot start an instance of that architecture) by
	// using an architecture constraint; this will have the effect of
	// limiting the available tools to just those matching the specified
	// architecture.
	Bootstrap(ctx BootstrapContext, params BootstrapParams) (arch, series string, _ BootstrapFinalizer, _ error)

	// InstanceBroker defines methods for starting and stopping
	// instances.
	InstanceBroker

	// AllocateAddress requests a specific address to be allocated for the
	// given instance on the given network.
	AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error

	// ReleaseAddress releases a specific address previously allocated with
	// AllocateAddress.
	ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address) error

	// Subnets returns basic information about all subnets known
	// by the provider for the environment, for a specific instance. A
	// provider may return all networks instead of just those for the
	// instance (provider specific).
	Subnets(inst instance.Id) ([]network.SubnetInfo, error)

	// ConfigGetter allows the retrieval of the configuration data.
	ConfigGetter

	// EnvironCapability allows access to this environment's capabilities.
	state.EnvironCapability

	// ConstraintsValidator returns a Validator instance which
	// is used to validate and merge constraints.
	ConstraintsValidator() (constraints.Validator, error)

	// SetConfig updates the Environ's configuration.
	//
	// Calls to SetConfig do not affect the configuration of
	// values previously obtained from Storage.
	SetConfig(cfg *config.Config) error

	// Instances returns a slice of instances corresponding to the
	// given instance ids.  If no instances were found, but there
	// was no other error, it will return ErrNoInstances.  If
	// some but not all the instances were found, the returned slice
	// will have some nil slots, and an ErrPartialInstances error
	// will be returned.
	Instances(ids []instance.Id) ([]instance.Instance, error)

	// StateServerInstances returns the IDs of instances corresponding
	// to Juju state servers. If there are no state server instances,
	// ErrNotBootstrapped is returned.
	StateServerInstances() ([]instance.Id, error)

	// Destroy shuts down all known machines and destroys the
	// rest of the environment. Note that on some providers,
	// very recently started instances may not be destroyed
	// because they are not yet visible.
	//
	// When Destroy has been called, any Environ referring to the
	// same remote environment may become invalid
	Destroy() error

	// OpenPorts opens the given port ranges for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	OpenPorts(ports []network.PortRange) error

	// ClosePorts closes the given port ranges for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	ClosePorts(ports []network.PortRange) error

	// Ports returns the port ranges opened for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	Ports() ([]network.PortRange, error)

	// Provider returns the EnvironProvider that created this Environ.
	Provider() EnvironProvider

	state.Prechecker
}

// BootstrapContext is an interface that is passed to
// Environ.Bootstrap, providing a means of obtaining
// information about and manipulating the context in which
// it is being invoked.
type BootstrapContext interface {
	GetStdin() io.Reader
	GetStdout() io.Writer
	GetStderr() io.Writer
	Infof(format string, params ...interface{})
	Verbosef(format string, params ...interface{})

	// InterruptNotify starts watching for interrupt signals
	// on behalf of the caller, sending them to the supplied
	// channel.
	InterruptNotify(sig chan<- os.Signal)

	// StopInterruptNotify undoes the effects of a previous
	// call to InterruptNotify with the same channel. After
	// StopInterruptNotify returns, no more signals will be
	// delivered to the channel.
	StopInterruptNotify(chan<- os.Signal)

	// ShouldVerifyCredentials indicates whether the caller's cloud
	// credentials should be verified.
	ShouldVerifyCredentials() bool
}
