// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	"os"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
	"github.com/juju/errors"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envinstance "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/version"
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
	if ctx.ShouldVerifyCredentials() {
		logger.Infof("Loging into the oracle cloud infrastructure")
		if err := o.p.client.Authenticate(); err != nil {
			return errors.Trace(err)
		}
	}
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
func (o *oracleEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	params environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, o, params)
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
	if args.ControllerUUID == "" {
		return nil, errors.NotFoundf("Controller UUID")
	}

	// TODO(sgiulitti): for know adding additional arguments are not supported
	if args.Placement != "" {
		return nil, errors.NotSupportedf("Adding placements")
	}

	series := args.Tools.OneSeries()
	arches := args.Tools.Arches()

	types, err := getInstanceTypes(o.p.client)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if args.ImageMetadata, err = checkImageList(
		o.p.client, args.Constraints,
	); err != nil {
		return nil, errors.Trace(err)
	}

	//find the best suitable instance returned from the api
	spec, imagelist, err := findInstanceSpec(
		o.p.client,
		args.ImageMetadata,
		types,
		&envinstance.InstanceConstraint{
			Region:      o.spec.Region,
			Series:      series,
			Arches:      arches,
			Constraints: args.Constraints,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	tools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err = args.InstanceConfig.SetTools(tools); err != nil {
		return nil, errors.Trace(err)
	}

	if err = instancecfg.FinishInstanceConfig(args.InstanceConfig, o.Config()); err != nil {
		return nil, errors.Trace(err)
	}

	//TODO
	instance, err := createInstance(o.p.client, oci.InstanceParams{
		Relationships: nil,
		Instances: []oci.Instances{
			{
				Shape:     spec.InstanceType.Name,
				Imagelist: imagelist,
				Name:      args.InstanceConfig.MachineAgentServiceName,
				Label:     o.cfg.Name(),
				//SSHKeys:   []string{nameKey},
				Hostname: o.cfg.Name(),
				Tags:     []string{args.InstanceConfig.ControllerTag.String()},
				// TODO(sgiulitti): add here the userdata
				Attributes:  nil,
				Reverse_dns: false,
				// TODO(sgiulitti): make vm generate a public address
			},
		},
	})
	fmt.Println(instance)
	fmt.Println(err)
	os.Exit(1)
	if err != nil {
		return nil, err
	}

	result := &environs.StartInstanceResult{
		Instance: instance,
	}

	_ = instance
	return result, nil
}

// StopInstances shuts down the instances with the specified IDs.
// Unknown instance IDs are ignored, to enable idempotency.
func (o oracleEnviron) StopInstances(...instance.Id) error {
	return nil
}

// AllInstances returns all instances currently known to the broker.
func (o oracleEnviron) AllInstances() ([]instance.Instance, error) {
	resp, err := o.p.client.AllInstances()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(resp.Result) == 0 {
		return nil, environs.ErrNoInstances
	}

	all := make([]instance.Instance, 0, len(resp.Result))
	for _, val := range resp.Result {
		inst, err := newInstance(&val)
		if err != nil {
			return nil, errors.Trace(err)
		}
		all = append(all, inst)
	}

	return all, nil
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
		constraints.VirtType,
		constraints.Spaces,
	}

	// we choose to use the default validator implementation
	validator := constraints.NewValidator()
	// we must feed the validator that the oracle cloud
	// provider does not support these constraints
	validator.RegisterUnsupported(unsupportedConstraints)

	return validator, nil
}

// SetConfig updates the Environ's configuration.
//
// Calls to SetConfig do not affect the configuration of
// values previously obtained from Storage.
func (o *oracleEnviron) SetConfig(cfg *config.Config) error {
	var old *config.Config
	if o.cfg != nil {
		old = o.cfg
	}
	if err := config.Validate(cfg, old); err != nil {
		return errors.Trace(err)
	}
	o.cfg = cfg
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
