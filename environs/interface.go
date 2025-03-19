// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"
	"io"

	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	"github.com/juju/version/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/assumes"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network/firewall"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/proxy"
	"github.com/juju/juju/internal/storage"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/package_mock.go -write_package_comment=false github.com/juju/juju/environs EnvironProvider,CloudEnvironProvider,ProviderSchema,ProviderCredentials,FinalizeCredentialContext,FinalizeCloudContext,CloudFinalizer,CloudDetector,CloudRegionDetector,ConfigGetter,CloudDestroyer,Environ,InstancePrechecker,Firewaller,InstanceTagger,InstanceTypesFetcher,Upgrader,UpgradeStep,DefaultConstraintsChecker,ProviderCredentialsRegister,RequestFinalizeCredential,NetworkingEnviron

type ConnectorInfo interface {
	ConnectionProxyInfo(ctx context.Context) (proxy.Proxier, error)
}

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
	Ping(ctx context.Context, endpoint string) error

	// ValidateCloud returns an error if the supplied cloud spec is not
	// valid for use by the provider. This is called for the controller
	// model during bootstrap, and also for new hosted models.
	ValidateCloud(context.Context, environscloudspec.CloudSpec) error
}

// ModelConfigProvider represents an interface that a [EnvironProvider] can
// implement to provide opinions and defaults into a model's config.
type ModelConfigProvider interface {
	// ConfigDefaults returns the default values for the
	// provider specific config attributes.
	ConfigDefaults() schema.Defaults

	// ConfigSchema returns extra config attributes specific
	// to this provider only.
	ConfigSchema() schema.Fields

	// ModelConfigDefaults provides a set of default model config attributes
	// that should be set on a models config if they have not been specified by
	// the user.
	ModelConfigDefaults(context.Context) (map[string]any, error)
}

// A CloudEnvironProvider represents a computing and storage provider
// for a traditional cloud like AWS or Openstack.
type CloudEnvironProvider interface {
	EnvironProvider
	// Open opens the environment and returns it. The configuration must
	// have passed through PrepareConfig at some point in its lifecycle.
	//
	// Open should not perform any expensive operations, such as querying
	// the cloud API, as it will be called frequently.
	Open(context.Context, OpenParams, CredentialInvalidator) (Environ, error)
}

// OpenParams contains the parameters for EnvironProvider.Open.
type OpenParams struct {
	// ControllerUUID is the controller UUID.
	ControllerUUID string

	// Cloud is the cloud specification to use to connect to the cloud.
	Cloud environscloudspec.CloudSpec

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
	Schema() configschema.Fields
}

// ProviderCredentials is an interface that an EnvironProvider implements
// in order to validate and automatically detect credentials for clouds
// supported by the provider.
//
// TODO(axw) replace CredentialSchemas with an updated configschema.
// The Dashboard also needs to be able to handle multiple credential types,
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
	//
	// If cloud name is not passed (empty-string), all credentials are
	// returned, otherwise only the credential for that cloud.
	DetectCredentials(cloudName string) (*cloud.CloudCredential, error)

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

// BootstrapCredentialsFinaliser is an interface for environs to provide a
// method for finalizing bootstrap credentials before being passed to a
// newly bootstrapped controller.
type BootstrapCredentialsFinaliser interface {
	// FinalizeBootstrapCredential finalizes credential as the last step of a
	// bootstrap process.
	FinaliseBootstrapCredential(BootstrapContext, BootstrapParams, *cloud.Credential) (*cloud.Credential, error)
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
	RegisterCredentials(cloud.Cloud) (map[string]*cloud.CloudCredential, error)
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

	// Verbosef will write the formatted string to Stderr if the
	// verbose flag is true, and to the logger if not.
	Verbosef(string, ...interface{})
}

// FinalizeCredentialParams contains the parameters for
// ProviderCredentials.FinalizeCredential.
type FinalizeCredentialParams struct {
	// Credential is the credential that the provider should finalize.
	Credential cloud.Credential

	// CloudName is the name of the cloud that the credentials are for.
	CloudName string

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
type FinalizeCloudContext interface {
	context.Context

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

// ConfigGetter implements access to an environment's configuration.
type ConfigGetter interface {
	// Config returns the configuration data with which the Environ was created.
	// Note that this is not necessarily current; the canonical location
	// for the configuration data is stored in the state.
	Config() *config.Config
}

// ConfigSetter implements access to an environment's configuration.
type ConfigSetter interface {
	// SetConfig updates the Environ's configuration.
	//
	// Calls to SetConfig do not affect the configuration of
	// values previously obtained from Storage.
	SetConfig(ctx context.Context, cfg *config.Config) error
}

// CloudSpecSetter implements access to an environment's cloud spec.
type CloudSpecSetter interface {
	// SetCloudSpec updates the Environ's configuration.
	SetCloudSpec(ctx context.Context, spec environscloudspec.CloudSpec) error
}

// Bootstrapper provides the way for bootstrapping controller.
type Bootstrapper interface {
	// PrepareForBootstrap will be called very early in the bootstrap
	// procedure to give an Environ a chance to perform interactive
	// operations that are required for bootstrapping.
	PrepareForBootstrap(ctx BootstrapContext, controllerName string) error

	// Bootstrap creates a new environment, and an instance to host the
	// controller for that environment. The instance will have the
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
	Bootstrap(ctx BootstrapContext, callCtx envcontext.ProviderCallContext, params BootstrapParams) (*BootstrapResult, error)
}

// Configer implements access to an environment's configuration.
type Configer interface {
	ConfigGetter
	ConfigSetter
}

// BootstrapEnviron is an interface that an EnvironProvider implements
// in order to bootstrap a controller.
type BootstrapEnviron interface {
	Configer
	Bootstrapper
	ConstraintsChecker

	CloudDestroyer
	ControllerDestroyer

	// ProviderRegistry is implemented in order to acquire
	// environ-scoped storage providers supported by the Environ.
	// StorageProviders returned from Environ.StorageProvider will
	// be scoped specifically to that Environ.
	storage.ProviderRegistry

	// Create creates the environment for a new hosted model.
	//
	// This will be called before any workers begin operating on the
	// Environ, to give an Environ a chance to perform operations that
	// are required for further use.
	//
	// Create is not called for the initial controller model; it is
	// the Bootstrap method's job to create the controller model.
	Create(envcontext.ProviderCallContext, CreateParams) error
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
	Destroy(ctx envcontext.ProviderCallContext) error
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
	BootstrapEnviron

	// ResourceAdopter defines methods for adopting resources.
	ResourceAdopter

	// InstanceBroker defines methods for starting and stopping
	// instances.
	InstanceBroker

	InstanceLister

	// ControllerInstances returns the IDs of instances corresponding
	// to Juju controller, having the specified controller UUID.
	// If there are no controller instances, ErrNoInstances is returned.
	// If it can be determined that the environment has not been bootstrapped,
	// then ErrNotBootstrapped should be returned instead.
	ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error)

	// Provider returns the EnvironProvider that created this Environ.
	Provider() EnvironProvider

	InstancePrechecker

	// InstanceTypesFetcher represents an environment that can return
	// information about the available instance types.
	InstanceTypesFetcher
}

// ControllerDestroyer is similar to Destroy() in that it destroys
// the model, which in this case will be the controller model.
//
// In addition, this method also destroys any resources relating
// to hosted models on the controller on which it is invoked.
// This ensures that "kill-controller" can clean up hosted models
// when the Juju controller process is unavailable.
type ControllerDestroyer interface {
	DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error
}

type ResourceAdopter interface {
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
	AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error
}

// ConstraintsChecker provides a means to check that constraints are valid.
type ConstraintsChecker interface {
	// ConstraintsValidator returns a Validator instance which
	// is used to validate and merge constraints.
	ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error)
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
	PrecheckInstance(envcontext.ProviderCallContext, PrecheckInstanceParams) error
}

// InstanceLister provider api to list instances for specified instance ids.
type InstanceLister interface {
	// Instances returns a slice of instances corresponding to the
	// given instance ids.  If no instances were found, but there
	// was no other error, it will return ErrNoInstances.  If
	// some but not all the instances were found, the returned slice
	// will have some nil slots, and an ErrPartialInstances error
	// will be returned.
	Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error)
}

// PrecheckInstanceParams contains the parameters for
// InstancePrechecker.PrecheckInstance.
type PrecheckInstanceParams struct {
	// Base contains the base of the machine.
	Base corebase.Base

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
	OpenPorts(ctx envcontext.ProviderCallContext, rules firewall.IngressRules) error

	// ClosePorts closes the given port ranges for the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	ClosePorts(ctx envcontext.ProviderCallContext, rules firewall.IngressRules) error

	// IngressRules returns the ingress rules applied to the whole environment.
	// Must only be used if the environment was setup with the
	// FwGlobal firewall mode.
	// It is expected that there be only one ingress rule result for a given
	// port range - the rule's SourceCIDRs will contain all applicable source
	// address rules for that port range.
	IngressRules(ctx envcontext.ProviderCallContext) (firewall.IngressRules, error)
}

// FirewallFeatureQuerier exposes methods for detecting what features the
// environment firewall supports.
type FirewallFeatureQuerier interface {
	// SupportsRulesWithIPV6CIDRs returns true if the environment supports
	// ingress rules containing IPV6 CIDRs.
	SupportsRulesWithIPV6CIDRs(ctx envcontext.ProviderCallContext) (bool, error)
}

// InstanceTagger is an interface that can be used for tagging instances.
type InstanceTagger interface {
	// TagInstance tags the given instance with the specified tags.
	//
	// The specified tags will replace any existing ones with the
	// same names, but other existing tags will be left alone.
	TagInstance(ctx envcontext.ProviderCallContext, id instance.Id, tags map[string]string) error
}

// InstanceTypesFetcher is an interface that allows for instance information from
// a provider to be obtained.
type InstanceTypesFetcher interface {
	InstanceTypes(context.Context, constraints.Value) (instances.InstanceTypesWithCostMetadata, error)
}

// Upgrader is an interface that can be used for upgrading Environs. If an
// Environ implements this interface, its UpgradeOperations method will be
// invoked to identify operations that should be run on upgrade.
type Upgrader interface {
	// UpgradeOperations returns a list of UpgradeOperations for upgrading
	// an Environ.
	UpgradeOperations(envcontext.ProviderCallContext, UpgradeOperationsParams) []UpgradeOperation
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
	Run(ctx envcontext.ProviderCallContext) error
}

// JujuUpgradePrechecker is an interface that can be used to precheck
// the Environs before upgrading juju. If an Environ implements this
// interface, its PrecheckUpgradeOperations method will be invoked to
// identify operations that should be run to check if an juju upgrade
// is possible.
type JujuUpgradePrechecker interface {
	// PreparePrechecker is called to to give an Environ a chance to
	// perform interactive operations that are required for prechecking
	// an upgrade.
	PreparePrechecker() error

	// PrecheckUpgradeOperations returns a list of
	// PrecheckJujuUpgradeOperations for checking if juju can be upgrade.
	PrecheckUpgradeOperations() []PrecheckJujuUpgradeOperation
}

// PrecheckJujuUpgradeOperation contains a target agent version and
// sequence of upgrade precheck steps to apply to get to that version.
type PrecheckJujuUpgradeOperation struct {
	// TargetVersion is the target juju version.
	TargetVersion version.Number

	// Steps contains the sequence of upgrade steps to apply when
	// upgrading to the accompanying target version number.
	Steps []PrecheckJujuUpgradeStep
}

// PrecheckJujuUpgradeStep defines an idempotent operation that is run
// a specific precheck upgrade step on an Environ.
type PrecheckJujuUpgradeStep interface {
	// Description is a human readable description of what the
	// precheck upgrade step does.
	Description() string

	// Run executes the precheck upgrade business logic.
	Run() error
}

// DefaultConstraintsChecker defines an interface for checking if the default
// constraints should be applied for the Environ provider when bootstrapping
// the provider.
type DefaultConstraintsChecker interface {
	// ShouldApplyControllerConstraints returns if bootstrapping logic should
	// use default constraints
	ShouldApplyControllerConstraints(constraints.Value) bool
}

// HardwareCharacteristicsDetector is implemented by environments that can
// provide advance information about the series and hardware for controller
// instances that have not been provisioned yet.
type HardwareCharacteristicsDetector interface {
	// DetectBase returns the base for the controller instance.
	DetectBase() (corebase.Base, error)
	// DetectHardware returns the hardware characteristics for the
	// controller instance.
	DetectHardware() (*instance.HardwareCharacteristics, error)
	// UpdateModelConstraints returns true if the model constraints should
	// be updated based on the returns of DetectBase() and
	// DetectHardware().
	UpdateModelConstraints() bool
}

// SupportedFeatureEnumerator is implemented by environments that can report
// the set of features that are available to charms deployed to them.
type SupportedFeatureEnumerator interface {
	SupportedFeatures() (assumes.FeatureSet, error)
}

// CheckProvider defines the old and/or public cloud style of cloud
// endpoint validation.  This check is a heavy weight method to
// verify the current cloud connectivity.
// Typically used with public clouds which have not implemented the
// CloudEndpointChecker.
type CheckProvider interface {
	// AllInstances returns all instances currently known to the broker.
	AllInstances(ctx context.Context) ([]instances.Instance, error)
}

// CloudEndpointChecker defines a method for cloud endpoint validation.
//
// TODO: hml 09-Feb-22
// Implement this interface for all providers, including the public
// clouds.
type CloudEndpointChecker interface {
	// ValidateCloudEndpoint validates connectivity with the cloud's
	// endpoint and returns nil if no problems.
	ValidateCloudEndpoint(ctx context.Context) error
}
