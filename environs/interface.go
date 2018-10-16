// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"io"

	"github.com/juju/jsonschema"
	"github.com/juju/version"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
)

//go:generate mockgen -package testing -destination testing/package_mock.go github.com/juju/juju/environs EnvironProvider,CloudEnvironProvider,ProviderSchema,ProviderCredentials,FinalizeCredentialContext,FinalizeCloudContext,CloudFinalizer,CloudDetector,CloudRegionDetector,ModelConfigUpgrader,ConfigGetter,CloudDestroyer,Environ,InstancePrechecker,Firewaller,InstanceTagger,InstanceTypesFetcher,Upgrader,UpgradeStep,DefaultConstraintsChecker,ProviderCredentialsRegister,RequestFinalizeCredential,NetworkingEnviron

// A EnvironProvider represents a computing and storage provider
// for either a traditional cloud or a container substrate like k8s.
type EnvironProvider interface {
	config.Validator
	ProviderCredentials

	// Version returns the version of the provider. This is recorded as the
	// environ version for each model, and used to identify which upgrade
	// operations to run when upgrading a model's environ. Providers should
	// start out at version 0.
	Version() int

	// CloudSchema returns the schema used to validate input for add-cloud.  If
	// a provider does not support custom clouds, CloudSchema should return
	// nil.
	CloudSchema() *jsonschema.Schema

	// Ping tests the connection to the cloud, to verify the endpoint is valid.
	Ping(ctx context.ProviderCallContext, endpoint string) error

	// PrepareConfig prepares the configuration for a new model, based on
	// the provided arguments. PrepareConfig is expected to produce a
	// deterministic output. Any unique values should be based on the
	// "uuid" attribute of the base configuration. This is called for the
	// controller model during bootstrap, and also for new hosted models.
	PrepareConfig(PrepareConfigParams) (*config.Config, error)
}

// A EnvironProvider represents a computing and storage provider
// for a traditional cloud like AWS or Openstack.
type CloudEnvironProvider interface {
	EnvironProvider
	// Open opens the environment and returns it. The configuration must
	// have passed through PrepareConfig at some point in its lifecycle.
	//
	// Open should not perform any expensive operations, such as querying
	// the cloud API, as it will be called frequently.
	Open(OpenParams) (Environ, error)
}

// OpenParams contains the parameters for EnvironProvider.Open.
type OpenParams struct {
	// Cloud is the cloud specification to use to connect to the cloud.
	Cloud CloudSpec

	// Config is the base configuration for the provider.
	Config *config.Config
}

// ProviderSchema can be implemented by a provider to provide
// access to its configuration schema. Once all providers implement
// this, it will be included in the EnvironProvider type and the
// information made available over the API.
type ProviderSchema interface {
	// Schema returns the schema for the provider. It should
	// include all fields defined in environs/config, conventionally
	// by calling config.Schema.
	Schema() environschema.Fields
}

// PrepareConfigParams contains the parameters for EnvironProvider.PrepareConfig.
type PrepareConfigParams struct {
	// Cloud is the cloud specification to use to connect to the cloud.
	Cloud CloudSpec

	// Config is the base configuration for the provider. This should
	// be updated with the region, endpoint and credentials.
	Config *config.Config
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

	// FinalizeCredential finalizes a credential, updating any attributes
	// as necessary. This is always done client-side, when adding the
	// credential to credentials.yaml and before uploading credentials to
	// the controller. The provider may completely alter a credential, even
	// going as far as changing the auth-type, but the output must be a
	// fully formed credential.
	FinalizeCredential(
		FinalizeCredentialContext,
		FinalizeCredentialParams,
	) (*cloud.Credential, error)
}

// ProviderCredentialsRegister is an interface that an EnvironProvider
// implements in order to validate and automatically register credentials for
// clouds supported by the provider.
type ProviderCredentialsRegister interface {

	// RegisterCredentials will return any credentials that need to be
	// registered for the provider.
	//
	// If no credentials can be found, RegisterCredentials should return
	// an error satisfying errors.IsNotFound.
	RegisterCredentials() (map[string]*cloud.CloudCredential, error)
}

// RequestFinalizeCredential is an interface that an EnvironProvider implements
// in order to call ProviderCredentials.FinalizeCredential strictly rather than
// lazily to gather fully formed credentials.
type RequestFinalizeCredential interface {

	// ShouldFinalizeCredential asks if a EnvironProvider wants to strictly
	// finalize a credential. The provider just returns true if they want to
	// call FinalizeCredential from ProviderCredentials when asked.
	ShouldFinalizeCredential(cloud.Credential) bool
}

// FinalizeCredentialContext is an interface passed into FinalizeCredential
// to provide a means of interacting with the user when finalizing credentials.
type FinalizeCredentialContext interface {
	GetStderr() io.Writer
}

// FinalizeCredentialParams contains the parameters for
// ProviderCredentials.FinalizeCredential.
type FinalizeCredentialParams struct {
	// Credential is the credential that the provider should finalize.
	Credential cloud.Credential

	// CloudEndpoint is the endpoint for the cloud that the credentials are
	// for. This may be used by the provider to communicate with the cloud
	// to finalize the credentials.
	CloudEndpoint string

	// CloudStorageEndpoint is the storage endpoint for the cloud that the
	// credentials are for. This may be used by the provider to communicate
	// with the cloud to finalize the credentials.
	CloudStorageEndpoint string

	// CloudIdentityEndpoint is the identity endpoint for the cloud that the
	// credentials are for. This may be used by the provider to communicate
	// with the cloud to finalize the credentials.
	CloudIdentityEndpoint string
}

// FinalizeCloudContext is an interface passed into FinalizeCloud
// to provide a means of interacting with the user when finalizing
// a cloud definition.
//go:generate mockgen -package testing -destination testing/cloud_context_mock.go github.com/juju/juju/environs FinalizeCloudContext
type FinalizeCloudContext interface {
	// Verbosef will write the formatted string to Stderr if the
	// verbose flag is true, and to the logger if not.
	Verbosef(string, ...interface{})
}

// CloudFinalizer is an interface that an EnvironProvider implements
// in order to finalize a cloud.Cloud definition before bootstrapping.
type CloudFinalizer interface {
	// FinalizeCloud finalizes a cloud definition, updating any attributes
	// as necessary. This is always done client-side, before bootstrapping.
	FinalizeCloud(FinalizeCloudContext, cloud.Cloud) (cloud.Cloud, error)
}

// CloudDetector is an interface that an EnvironProvider implements
// in order to automatically detect clouds from the environment.
type CloudDetector interface {
	// DetectCloud attempts to detect a cloud with the given name
	// from the environment. This may involve, for example,
	// inspecting environment variables, or returning special
	// hard-coded regions (e.g. "localhost" for lxd).
	//
	// If no cloud can be detected, DetectCloud should return
	// an error satisfying errors.IsNotFound.
	//
	// DetectCloud should be used in preference to DetectClouds
	// when a specific cloud is identified, as this may be more
	// efficient.
	DetectCloud(name string) (cloud.Cloud, error)

	// DetectClouds detects clouds from the environment. This may
	// involve, for example, inspecting environment variables, or
	// returning special hard-coded regions (e.g. "localhost" for lxd).
	DetectClouds() ([]cloud.Cloud, error)
}

// CloudRegionDetector is an interface that an EnvironProvider implements
// in order to automatically detect cloud regions from the environment.
type CloudRegionDetector interface {
	// DetectRegions automatically detects one or more regions
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
	// TODO (anastasiamac 2018-04-27) Since it is only for 1.25, do we still need it?
	UpgradeConfig(cfg *config.Config) (*config.Config, error)
}

// ConfigGetter implements access to an environment's configuration.
type ConfigGetter interface {
	// Config returns the configuration data with which the Environ was created.
	// Note that this is not necessarily current; the canonical location
	// for the configuration data is stored in the state.
	Config() *config.Config
}

// CloudDestroyer provides the API to cleanup cloud resources.
type CloudDestroyer interface {
	// Destroy shuts down all known machines and destroys the
	// rest of the environment. Note that on some providers,
	// very recently started instances may not be destroyed
	// because they are not yet visible.
	//
	// When Destroy has been called, any Environ referring to the
	// same remote environment may become invalid.
	Destroy(ctx context.ProviderCallContext) error
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
	// Environ implements storage.ProviderRegistry for acquiring
	// environ-scoped storage providers supported by the Environ.
	// StorageProviders returned from Environ.StorageProvider will
	// be scoped specifically to that Environ.
	storage.ProviderRegistry

	// CloudDestroyer provides the API to cleanup cloud resources.
	CloudDestroyer

	// PrepareForBootstrap prepares an environment for bootstrapping.
	//
	// This will be called very early in the bootstrap procedure, to
	// give an Environ a chance to perform interactive operations that
	// are required for bootstrapping.
	PrepareForBootstrap(ctx BootstrapContext) error

	// Bootstrap creates a new environment, and an instance to host the
	// controller for that environment. The instnace will have have the
	// series and architecture of the Environ's choice, constrained to
	// those of the available tools. Bootstrap will return the instance's
	// architecture, series, and a function that must be called to finalize
	// the bootstrap process by transferring the tools and installing the
	// initial Juju controller.
	//
	// It is possible to direct Bootstrap to use a specific architecture
	// (or fail if it cannot start an instance of that architecture) by
	// using an architecture constraint; this will have the effect of
	// limiting the available tools to just those matching the specified
	// architecture.
	Bootstrap(ctx BootstrapContext, callCtx context.ProviderCallContext, params BootstrapParams) (*BootstrapResult, error)

	// Create creates the environment for a new hosted model.
	//
	// This will be called before any workers begin operating on the
	// Environ, to give an Environ a chance to perform operations that
	// are required for further use.
	//
	// Create is not called for the initial controller model; it is
	// the Bootstrap method's job to create the controller model.
	Create(context.ProviderCallContext, CreateParams) error

	// AdoptResources is called when the model is moved from one
	// controller to another using model migration. Some providers tag
	// instances, disks, and cloud storage with the controller UUID to
	// aid in clean destruction. This method will be called on the
	// environ for the target controller so it can update the
	// controller tags for all of those things. For providers that do
	// not track the controller UUID, a simple method returning nil
	// will suffice. The version number of the source controller is
	// provided for backwards compatibility - if the technique used to
	// tag items changes, the version number can be used to decide how
	// to remove the old tags correctly.
	AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error

	// InstanceBroker defines methods for starting and stopping
	// instances.
	InstanceBroker

	// ConfigGetter allows the retrieval of the configuration data.
	ConfigGetter

	// ConstraintsChecker provides a means to check that constraints are valid.
	ConstraintsChecker

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
	Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instance.Instance, error)

	// ControllerInstances returns the IDs of instances corresponding
	// to Juju controller, having the specified controller UUID.
	// If there are no controller instances, ErrNoInstances is returned.
	// If it can be determined that the environment has not been bootstrapped,
	// then ErrNotBootstrapped should be returned instead.
	ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error)

	// DestroyController is similar to Destroy() in that it destroys
	// the model, which in this case will be the controller model.
	//
	// In addition, this method also destroys any resources relating
	// to hosted models on the controller on which it is invoked.
	// This ensures that "kill-controller" can clean up hosted models
	// when the Juju controller process is unavailable.
	DestroyController(ctx context.ProviderCallContext, controllerUUID string) error

	// Provider returns the EnvironProvider that created this Environ.
	Provider() EnvironProvider

	InstancePrechecker

	// InstanceTypesFetcher represents an environment that can return
	// information about the available instance types.
	InstanceTypesFetcher
}

// ConstraintsChecker provides a means to check that constraints are valid.
type ConstraintsChecker interface {
	// ConstraintsValidator returns a Validator instance which
	// is used to validate and merge constraints.
	ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error)
}

// InstancePrechecker provides a means of "prechecking" instance
// arguments before recording them in state.
type InstancePrechecker interface {
	// PrecheckInstance performs a preflight check on the specified
	// series and constraints, ensuring that they are possibly valid for
	// creating an instance in this model.
	//
	// PrecheckInstance is best effort, and not guaranteed to eliminate
	// all invalid parameters. If PrecheckInstance returns nil, it is not
	// guaranteed that the constraints are valid; if a non-nil error is
	// returned, then the constraints are definitely invalid.
	PrecheckInstance(context.ProviderCallContext, PrecheckInstanceParams) error
}

// PrecheckInstanceParams contains the parameters for
// InstancePrechecker.PrecheckInstance.
type PrecheckInstanceParams struct {
	// Series contains the series of the machine.
	Series string

	// Constraints contains the machine constraints.
	Constraints constraints.Value

	// Placement contains the machine placement directive, if any.
	Placement string

	// VolumeAttachments contains the parameters for attaching existing
	// volumes to the instance. The PrecheckInstance method should not
	// expect the attachment's Machine field to be set, as PrecheckInstance
	// may be called before a machine ID is allocated.
	VolumeAttachments []storage.VolumeAttachmentParams
}

// CreateParams contains the parameters for Environ.Create.
type CreateParams struct {
	// ControllerUUID is the UUID of the controller to be that is creating
	// the Environ.
	ControllerUUID string
}

// Firewaller exposes methods for managing network ports.
type Firewaller interface {
	// OpenPorts opens the given port ranges for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	OpenPorts(ctx context.ProviderCallContext, rules []network.IngressRule) error

	// ClosePorts closes the given port ranges for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	ClosePorts(ctx context.ProviderCallContext, rules []network.IngressRule) error

	// IngressRules returns the ingress rules applied to the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	// It is expected that there be only one ingress rule result for a given
	// port range - the rule's SourceCIDRs will contain all applicable source
	// address rules for that port range.
	IngressRules(ctx context.ProviderCallContext) ([]network.IngressRule, error)
}

// InstanceTagger is an interface that can be used for tagging instances.
type InstanceTagger interface {
	// TagInstance tags the given instance with the specified tags.
	//
	// The specified tags will replace any existing ones with the
	// same names, but other existing tags will be left alone.
	TagInstance(ctx context.ProviderCallContext, id instance.Id, tags map[string]string) error
}

// InstanceTypesFetcher is an interface that allows for instance information from
// a provider to be obtained.
type InstanceTypesFetcher interface {
	InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error)
}

// Upgrader is an interface that can be used for upgrading Environs. If an
// Environ implements this interface, its UpgradeOperations method will be
// invoked to identify operations that should be run on upgrade.
type Upgrader interface {
	// UpgradeOperations returns a list of UpgradeOperations for upgrading
	// an Environ.
	UpgradeOperations(context.ProviderCallContext, UpgradeOperationsParams) []UpgradeOperation
}

// UpgradeOperationsParams contains the parameters for
// Upgrader.UpgradeOperations.
type UpgradeOperationsParams struct {
	// ControllerUUID is the UUID of the controller that manages
	// the Environ being upgraded.
	ControllerUUID string
}

// UpgradeOperation contains a target agent version and sequence of upgrade
// steps to apply to get to that version.
type UpgradeOperation struct {
	// TargetVersion is the target environ provider version number to
	// which the upgrade steps pertain. When a model is upgraded, all
	// upgrade operations will be run for versions greater than the
	// recorded environ version. This version number is independent of
	// the agent and controller versions.
	TargetVersion int

	// Steps contains the sequence of upgrade steps to apply when
	// upgrading to the accompanying target version number.
	Steps []UpgradeStep
}

// UpgradeStep defines an idempotent operation that is run to perform a
// specific upgrade step on an Environ.
type UpgradeStep interface {
	// Description is a human readable description of what the upgrade
	// step does.
	Description() string

	// Run executes the upgrade business logic.
	Run(ctx context.ProviderCallContext) error
}

// DefaultConstraintsChecker defines an interface for checking if the default
// constraints should be applied for the Environ provider when bootstrapping
// the provider.
type DefaultConstraintsChecker interface {
	// ShouldApplyControllerConstraints returns if bootstrapping logic should
	// use default constraints
	ShouldApplyControllerConstraints() bool
}
