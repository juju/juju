// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/juju/cloud"
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
	// be saved to be used later.
	PrepareForBootstrap(ctx BootstrapContext, args PrepareForBootstrapParams) (Environ, error)

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

	// SecretAttrs filters the supplied configuration returning only values
	// which are considered sensitive. All of the values of these secret
	// attributes need to be strings.
	SecretAttrs(cfg *config.Config) (map[string]string, error)

	ProviderCredentials
}

// PrepareForBootstrapParams contains the parameters for
// EnvironProvider.PrepareForBootstrap.
type PrepareForBootstrapParams struct {
	// Config is the base configuration for the provider. This should
	// be updated with the region, endpoint and credentials.
	Config *config.Config

	// Credentials is the set of credentials to use to bootstrap.
	Credentials cloud.Credential

	// CloudRegion is the name of the region of the cloud to create
	// the Juju controller in. This will be empty for clouds without
	// regions.
	CloudRegion string

	// CloudEndpoint is the location of the primary API endpoint to
	// use when communicating with the cloud.
	CloudEndpoint string

	// CloudStorageEndpoint is the location of the API endpoint to use
	// when communicating with the cloud's storage service. This will
	// be empty for clouds that have no cloud-specific API endpoint.
	CloudStorageEndpoint string
}

// ProviderCredentials is an interface that an EnvironProvider implements
// in order to validate and automatically detect credentials for clouds
// supported by the provider.
//
// TODO(axw) replace CredentialSchemas with an updated environschema.
// The GUI also needs to be able to handle multiple credential types,
// and dependencies in config attributes.
type ProviderCredentials interface {
	// CredentialSchemas returns credential schemas, keyed on
	// authentication type. These may be used to validate existing
	// credentials, or to generate new ones (e.g. to create an
	// interactive form.)
	CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema

	// DetectCredentials automatically detects one or more credentials
	// from the environment. This may involve, for example, inspecting
	// environment variables, or reading configuration files in
	// well-defined locations.
	//
	// If no credentials can be detected, DetectCredentials should
	// return an error satisfying errors.IsNotFound.
	DetectCredentials() (*cloud.CloudCredential, error)
}

// CloudRegionDetector is an interface that an EnvironProvider implements
// in order to automatically detect cloud regions from the environment.
type CloudRegionDetector interface {
	// DetectRetions automatically detects one or more regions
	// from the environment. This may involve, for example, inspecting
	// environment variables, or returning special hard-coded regions
	// (e.g. "localhost" for lxd). The first item in the list will be
	// considered the default region for bootstrapping if the user
	// does not specify one.
	//
	// If no regions can be detected, DetectRegions should return
	// an error satisfying errors.IsNotFound.
	DetectRegions() ([]cloud.Region, error)
}

// ModelConfigUpgrader is an interface that an EnvironProvider may
// implement in order to modify environment configuration on agent upgrade.
type ModelConfigUpgrader interface {
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

// An Environ represents a Juju environment.
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
	// the tools and installing the initial Juju controller.
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

	// ControllerInstances returns the IDs of instances corresponding
	// to Juju controllers. If there are no controller instances,
	// ErrNoInstances is returned. If it can be determined that the
	// environment has not been bootstrapped, then ErrNotBootstrapped
	// should be returned instead.
	ControllerInstances() ([]instance.Id, error)

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
