// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// A EnvironProvider represents a computing and storage provider.
type EnvironProvider interface {
	// RestrictedConfigAttributes are provider specific attributes stored in
	// the config that really cannot or should not be changed across
	// environments running inside a single juju server.
	RestrictedConfigAttributes() []string

	// PrepareForCreateEnvironment prepares an environment for creation. Any
	// additional configuration attributes are added to the config passed in
	// and returned.  This allows providers to add additional required config
	// for new environments that may be created in an existing juju server.
	// Note that this is not called in a client context, so environment variables,
	// local files, etc are not available.
	PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error)

	// PrepareForBootstrap prepares an environment for use. Any additional
	// configuration attributes in the returned environment should
	// be saved to be used later. If the environment is already
	// prepared, this call is equivalent to Open.
	PrepareForBootstrap(ctx BootstrapContext, cfg *config.Config) (Environ, error)

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

// EnvironConfigUpgrader is an interface that an EnvironProvider may
// implement in order to modify environment configuration on agent upgrade.
type EnvironConfigUpgrader interface {
	// UpgradeConfig upgrades an old environment configuration by adding,
	// updating or removing attributes. UpgradeConfig must be idempotent,
	// as it may be called multiple times in the event of a partial upgrade.
	//
	// NOTE(axw) this is currently only called when upgrading to 1.25.
	// We should update the upgrade machinery to call this for every
	// version upgrade, so the upgrades package is not tightly coupled
	// to provider upgrades.
	UpgradeConfig(cfg *config.Config) (*config.Config, error)
}

// ConfigGetter implements access to an environment's configuration.
type ConfigGetter interface {
	// Config returns the configuration data with which the Environ was created.
	// Note that this is not necessarily current; the canonical location
	// for the configuration data is stored in the state.
	Config() *config.Config
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
	Bootstrap(ctx BootstrapContext, params BootstrapParams) (*BootstrapResult, error)

	// InstanceBroker defines methods for starting and stopping
	// instances.
	InstanceBroker

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
	// ErrNoInstances is returned. If it can be determined that the
	// environment has not been bootstrapped, then ErrNotBootstrapped
	// should be returned instead.
	StateServerInstances() ([]instance.Id, error)

	// Destroy shuts down all known machines and destroys the
	// rest of the environment. Note that on some providers,
	// very recently started instances may not be destroyed
	// because they are not yet visible.
	//
	// When Destroy has been called, any Environ referring to the
	// same remote environment may become invalid
	Destroy() error

	Firewaller

	// Provider returns the EnvironProvider that created this Environ.
	Provider() EnvironProvider

	state.Prechecker
}

// Firewaller exposes methods for managing network ports.
type Firewaller interface {
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
}

// InstanceTagger is an interface that can be used for tagging instances.
type InstanceTagger interface {
	// TagInstance tags the given instance with the specified tags.
	//
	// The specified tags will replace any existing ones with the
	// same names, but other existing tags will be left alone.
	TagInstance(id instance.Id, tags map[string]string) error
}
