package oracle

import (
	"strings"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envinstance "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
)

// oracleEnviron implements the environs.Environ interface
// and has behaviour specific that the interface provides.
type oracleEnviron struct {
	p    *environProvider
	spec environs.CloudSpec
	cfg  *config.Config
}

// newOracleEnviron returns a new oracleEnviron
// composed from a environProvider and args from environs.OpenParams,
// usually this method is used in Open method calls of the environProvider
func newOracleEnviron(p *environProvider, args environs.OpenParams) *oracleEnviron {
	env := &oracleEnviron{
		p:    p,
		spec: args.Cloud,
		cfg:  args.Config,
	}

	return env
}

// StorageProviderTypes returns the storage provider types
// contained within this registry.
//
// Determining the supported storage providers may be dynamic.
// Multiple calls for the same registry must return consistent
// results.
func (o oracleEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}

// StorageProvider returns the storage provider with the given
// provider type. StorageProvider must return an errors satisfying
// errors.IsNotFound if the registry does not contain said the
// specified provider type.
func (o oracleEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, nil
}

// PrepareForBootstrap prepares an environment for bootstrapping.
//
// This will be called very early in the bootstrap procedure, to
// give an Environ a chance to perform interactive operations that
// are required for bootstrapping.
func (o oracleEnviron) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	return nil
}

// Bootstrap creates a new environment, and an instance inside the
// oracle cloud infrastracture to host the controller for that
// environment. The instnace will have have the series and architecture
// of the Environ's choice, constrained to those of the available tools.
// Bootstrap will return the instance's
// architecture, series, and a function that must be called to finalize
// the bootstrap process by transferring the tools and installing the
// initial Juju controller.
//
// Bootstrap will use just one specific architecture because the oracle
// cloud only supports amd64.
func (o oracleEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	params environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	// in order too make the entire bootstrap process prossible
	// we must take into accounting some things:
	// validate if we have a shape based on the bootstrap constraints
	// and pick the right one
	// validate if we have in the imagelist a image correspoding to the
	// image tools specified.
	// validate if we have already the ssh keys and if we don't have them
	// upload them into the oracle infrstracture and enable the flag

	logger.Infof("Loging into the oracle cloud infrastructure")
	if err := o.p.client.Authenticate(); err != nil {
		return nil, errors.Trace(err)
	}

	// make api request to oracle cloud to give us a list of
	// all supported shapes
	shapes, err := o.p.client.AllShapeDetails()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// find the best suitable shape returned from the api
	shape, err := findShape(shapes.Result, params.BootstrapConstraints)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Infof(
		"Choosing the %s with %d cores and %d MB ram",
		shape.name, shape.cpus, shape.ram,
	)

	// make api request to oracle cloud to give us a list of iamges that are
	// stored there, if we don't have a image complaint with the metadata provided
	// by juju, if the shape picked up above is supported, we should bail out
	imagelist, err := checkImageList(o.p.client, params.ImageMetadata, shape)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// we are using just the juju ssh keys, no other keys
	keys := strings.Split(o.cfg.AuthorizedKeys(), "\n")
	// we should try to determine if we need to upload the keys or if
	// there is already the set of keys there if we found the ssh key
	// then we should check if the key has the flag enabled on true
	// if not, change it to true
	// if the key is not present in the oracle infrstracture then we should
	// make a request and upload it to make use for further bootstraping the
	// controller or adding other machines
	nameKey, err := uploadSSHControllerKeys(o.p.client, keys[0])
	if err != nil {
		return nil, errors.Trace(err)
	}
	instance, err := launchBootstrapConstroller(o.p.client, []oci.InstanceParams{
		{
			Shape:     shape.name,
			Imagelist: imagelist,
			Label:     o.cfg.UUID(),
			SSHKeys:   []string{nameKey},
			Name:      o.cfg.Name(),
		},
	})
	if err != nil {
		return nil, err
	}

	_ = instance
	return nil, nil
}

// BootstrapMessage optionally provides a message to be displayed to
// the user at bootstrap time.
func (o oracleEnviron) BootstrapMessage() string {
	return "SomeBootstrapMessage"
}

// Create creates the environment for a new hosted model.
//
// This will be called before any workers begin operating on the
// Environ, to give an Environ a chance to perform operations that
// are required for further use.
//
// Create is not called for the initial controller model; it is
// the Bootstrap method's job to create the controller model.
func (o oracleEnviron) Create(params environs.CreateParams) error {
	return nil
}

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
func (e oracleEnviron) AdoptResources(controllerUUID string, fromVersion version.Number) error {

	return nil
}

// StartInstance asks for a new instance to be created, associated with
// the provided config in machineConfig. The given config describes the juju
// state for the new instance to connect to. The config MachineNonce, which must be
// unique within an environment, is used by juju to protect against the
// consequences of multiple instances being started with the same machine
// id.
func (o oracleEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	return nil, nil
}

// StopInstances shuts down the instances with the specified IDs.
// Unknown instance IDs are ignored, to enable idempotency.
func (o oracleEnviron) StopInstances(...instance.Id) error {
	return nil
}

// AllInstances returns all instances currently known to the broker.
func (o oracleEnviron) AllInstances() ([]instance.Instance, error) {
	return nil, nil
}

// MaintainInstance is used to run actions on jujud startup for existing
// instances. It is currently only used to ensure that LXC hosts have the
// correct network configuration.
func (o oracleEnviron) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// Config returns the configuration data with which the Environ was created.
// Note that this is not necessarily current; the canonical location
// for the configuration data is stored in the state.
func (o oracleEnviron) Config() *config.Config {
	return o.cfg
}

// ConstraintsValidator returns a constraints.Validator instance which
// is used to validate and merge constraints.
//
// Validator defines operations on constraints attributes which are
// used to ensure a constraints value is valid, as well as being able
// to handle overridden attributes.
//
// This will use the default validator implementation from the constraints package.
func (o oracleEnviron) ConstraintsValidator() (constraints.Validator, error) {
	// list of unsupported oracle provider constraints
	unsupportedConstraints := []string{
		constraints.Container,
		constraints.CpuPower,
		constraints.RootDisk,
		constraints.Arch,
		constraints.InstanceType,
		constraints.VirtType,
		constraints.Spaces,
	}

	// we choose to use the default validator implementation
	validator := constraints.NewValidator()

	// we must feed the validator that the oracle cloud
	// provider does not support these constraints
	validator.RegisterUnsupported(unsupportedConstraints)

	return newConstraintsAdaptor(validator), nil
}

// SetConfig updates the Environ's configuration.
//
// Calls to SetConfig do not affect the configuration of
// values previously obtained from Storage.

func (o oracleEnviron) SetConfig(cfg *config.Config) error {
	return nil
}

// Instances returns a slice of instances corresponding to the
// given instance ids.  If no instances were found, but there
// was no other error, it will return ErrNoInstances.  If
// some but not all the instances were found, the returned slice
// will have some nil slots, and an ErrPartialInstances error
// will be returned.
func (o oracleEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return nil, nil
}

// ControllerInstances returns the IDs of instances corresponding
// to Juju controller, having the specified controller UUID.
// If there are no controller instances, ErrNoInstances is returned.
// If it can be determined that the environment has not been bootstrapped,
// then ErrNotBootstrapped should be returned instead.
func (o oracleEnviron) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	return nil, nil
}

// Destroy shuts down all known machines and destroys the
// rest of the environment. Note that on some providers,
// very recently started instances may not be destroyed
// because they are not yet visible.
//
// When Destroy has been called, any Environ referring to the
// same remote environment may become invalid.
func (o oracleEnviron) Destroy() error {
	return nil
}

// DestroyController is similar to Destroy() in that it destroys
// the model, which in this case will be the controller model.
//
// In addition, this method also destroys any resources relating
// to hosted models on the controller on which it is invoked.
// This ensures that "kill-controller" can clean up hosted models
// when the Juju controller process is unavailable.
func (o oracleEnviron) DestroyController(controllerUUID string) error {
	return nil
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (o oracleEnviron) OpenPorts(rules []network.IngressRule) error {
	return nil
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (o oracleEnviron) ClosePorts(rules []network.IngressRule) error {
	return nil
}

// IngressRules returns the ingress rules applied to the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
// It is expected that there be only one ingress rule result for a given
// port range - the rule's SourceCIDRs will contain all applicable source
// address rules for that port range.
func (o oracleEnviron) IngressRules() ([]network.IngressRule, error) {
	return nil, nil
}

// Provider returns the EnvironProvider that created this Environ.
func (o oracleEnviron) Provider() environs.EnvironProvider {
	return o.p
}

// PrecheckInstance performs a preflight check on the specified
// series and constraints, ensuring that they are possibly valid for
// creating an instance in this model.
//
// PrecheckInstance is best effort, and not guaranteed to eliminate
// all invalid parameters. If PrecheckInstance returns nil, it is not
// guaranteed that the constraints are valid; if a non-nil error is
// returned, then the constraints are definitely invalid.
//
// TODO(axw) find a home for state.Prechecker that isn't state and
// isn't environs, so both packages can refer to it. Maybe the
// constraints package? Can't be instance, because constraints
// import instance...
func (o oracleEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return nil
}

// InstanceTypes allows for instance information from a provider to be obtained.
func (o oracleEnviron) InstanceTypes(constraints.Value) (envinstance.InstanceTypesWithCostMetadata, error) {
	var i envinstance.InstanceTypesWithCostMetadata
	return i, nil
}

// Providing this methods oracleEnviron implements also the simplestreams.HasRegion
// interface
//
// Region returns the necessary attributes to uniquely identify this cloud instance.
func (o oracleEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   o.spec.Region,
		Endpoint: o.spec.Endpoint,
	}, nil
}

// Validate ensures that cfg is a valid configuration.
// If old is not nil, Validate should use it to determine
// whether a configuration change is valid.
//
// TODO(axw) Validate should just return an error. We should
// use a separate mechanism for updating config.
// func (o oracleEnviron) Validate(cfg, old *config.Config) (valid *config.Config, _ error) {
// 	return nil, nil
// }
