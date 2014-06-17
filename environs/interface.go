// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"io"
	"os"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
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

// EnvironStorage implements storage access for an environment
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
// Even though Juju takes care not to share an Environ between concurrent
// workers, it does allow concurrent method calls into the provider
// implementation.  The typical provider implementation needs locking to
// avoid undefined behaviour when the configuration changes.
type Environ interface {
	// Name returns the Environ's name.
	Name() string

	// Bootstrap initializes the state for the environment, possibly
	// starting one or more instances.  If the configuration's
	// AdminSecret is non-empty, the administrator password on the
	// newly bootstrapped state will be set to a hash of it (see
	// utils.PasswordHash), When first connecting to the
	// environment via the juju package, the password hash will be
	// automatically replaced by the real password.
	//
	// Bootstrap is responsible for selecting the appropriate tools,
	// and setting the agent-version configuration attribute prior to
	// bootstrapping the environment.
	Bootstrap(ctx BootstrapContext, params BootstrapParams) error

	// StateInfo returns information on the state initialized
	// by Bootstrap.
	StateInfo() (*state.Info, *api.Info, error)

	// InstanceBroker defines methods for starting and stopping
	// instances.
	InstanceBroker

	// AllocateAddress requests a new address to be allocated for the
	// given instance on the given network.
	AllocateAddress(instId instance.Id, netId network.Id) (network.Address, error)

	// ListNetworks returns basic information about all networks known
	// by the provider for the environment. They may be unknown to juju
	// yet (i.e. when called initially or when a new network was created).
	ListNetworks() ([]network.BasicInfo, error)

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

	EnvironStorage

	// Destroy shuts down all known machines and destroys the
	// rest of the environment. Note that on some providers,
	// very recently started instances may not be destroyed
	// because they are not yet visible.
	//
	// When Destroy has been called, any Environ referring to the
	// same remote environment may become invalid
	Destroy() error

	// OpenPorts opens the given ports for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	OpenPorts(ports []network.Port) error

	// ClosePorts closes the given ports for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	ClosePorts(ports []network.Port) error

	// Ports returns the ports opened for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	Ports() ([]network.Port, error)

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
}
