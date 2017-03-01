// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	// "os"
	"strings"
	"sync"
	"time"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
	// ociCommon "github.com/hoenirvili/go-oracle-cloud/common"
	"github.com/juju/errors"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envinstance "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools"
	"github.com/juju/version"
)

// oracleEnviron implements the environs.Environ interface
// and has behaviour specific that the interface provides.
type oracleEnviron struct {
	mutex *sync.Mutex
	p     *environProvider
	spec  environs.CloudSpec
	cfg   *config.Config
	// namespace instance.Namespace
}

// newOracleEnviron returns a new oracleEnviron
// composed from a environProvider and args from environs.OpenParams,
// usually this method is used in Open method calls of the environProvider
func newOracleEnviron(p *environProvider, args environs.OpenParams) *oracleEnviron {
	m := &sync.Mutex{}

	env := &oracleEnviron{
		p:     p,
		spec:  args.Cloud,
		cfg:   args.Config,
		mutex: m,
	}

	return env
}

// PrepareForBootstrap prepares an environment for bootstrapping.
//
// This will be called very early in the bootstrap procedure, to
// give an Environ a chance to perform interactive operations that
// are required for bootstrapping.
func (o *oracleEnviron) PrepareForBootstrap(ctx environs.BootstrapContext) error {
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
// func (o *oracleEnviron) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
// 	return common.Bootstrap(ctx, o, params)
// }

func (o *oracleEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, o, args)
}

// Create creates the environment for a new hosted model.
//
// This will be called before any workers begin operating on the
// Environ, to give an Environ a chance to perform operations that
// are required for further use.
//
// Create is not called for the initial controller model; it is
// the Bootstrap method's job to create the controller model.
func (o *oracleEnviron) Create(params environs.CreateParams) error {
	if err := o.p.client.Authenticate(); err != nil {
		return errors.Trace(err)
	}
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
func (e *oracleEnviron) AdoptResources(controllerUUID string, fromVersion version.Number) error {
	return nil
}

// StartInstance asks for a new instance to be created, associated with
// the provided config in machineConfig. The given config describes the juju
// state for the new instance to connect to. The config MachineNonce, which must be
// unique within an environment, is used by juju to protect against the
// consequences of multiple instances being started with the same machine
// id.
func (o *oracleEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	if args.ControllerUUID == "" {
		return nil, errors.NotFoundf("Controller UUID")
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
	logger.Tracef("Tools: %v", tools)
	if err = args.InstanceConfig.SetTools(tools); err != nil {
		return nil, errors.Trace(err)
	}

	if err = instancecfg.FinishInstanceConfig(args.InstanceConfig, o.Config()); err != nil {
		return nil, errors.Trace(err)
	}

	cloudcfg, err := cloudinit.New(args.InstanceConfig.Series)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}

	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, OracleRenderer{})
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("oracle user data: %d bytes", len(userData))

	attributes := map[string]interface{}{
		"userdata": string(userData),
	}

	tags := make([]string, 0, len(args.InstanceConfig.Tags))
	for k, v := range args.InstanceConfig.Tags {
		if k == "" || v == "" {
			continue
		}
		t := tagValue{k, v}
		tags = append(tags, t.String())
	}
	//TODO
	machineName := o.p.client.ComposeName(args.InstanceConfig.MachineAgentServiceName)
	imageName := o.p.client.ComposeName(imagelist)
	instance, err := createInstance(o.p.client, oci.InstanceParams{
		Relationships: nil,
		Instances: []oci.Instances{
			{
				Shape:       spec.InstanceType.Name,
				Imagelist:   imageName,
				Name:        machineName,
				Label:       args.InstanceConfig.MachineAgentServiceName,
				Hostname:    args.InstanceConfig.MachineAgentServiceName,
				Tags:        tags,
				Attributes:  attributes,
				Reverse_dns: false,
				// TODO(sgiulitti): make vm generate a public address
			},
		},
	})
	instance.arch = &spec.Image.Arch
	instance.instType = &spec.InstanceType

	if err != nil {
		return nil, err
	}

	machine := *instance.machine
	machineId := instance.machine.Name

	// wait for machine to start
	// TODO: cleanup all resources in case of failure.
	for !strings.EqualFold(machine.State, "running") {
		time.Sleep(1 * time.Second)

		logger.Tracef("Fetching instance details for %q", machineId)
		machine, err = o.p.client.InstanceDetails(machineId)
		if err != nil {
			return nil, errors.Annotate(err, "cannot start instances")
		}
	}
	logger.Infof("started instance %q", machineId)
	logger.Infof("Attempting to allocate public IP for instance %q", machineId)
	// tags should help us cleanup after ourselves
	reservationTags := []string{
		"juju",
	}
	reservation, err := o.p.client.CreateIpReservation(machineId, "", oci.PublicIPPool, true, reservationTags)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Infof("Associating public IP %q with instance %q", reservation.Name, machineId)

	assocPoolName := oci.NewIPPool(reservation.Name, oci.IPReservationType)
	_, err = o.p.client.CreateIpAssociation(
		assocPoolName,
		machine.Vcable_id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := &environs.StartInstanceResult{
		Instance: instance,
		Hardware: instance.hardwareCharacteristics(),
	}

	return result, nil
}

// StopInstances shuts down the instances with the specified IDs.
// Unknown instance IDs are ignored, to enable idempotency.
func (o *oracleEnviron) StopInstances(ids ...instance.Id) error {
	//TODO: delete security lists
	//TODO: delete public IP
	//TODO: delete storage volumes
	logger.Debugf("terminating instances %v", ids)
	if err := o.terminateInstances(ids...); err != nil {
		return err
	}
	return nil
}

func (o *oracleEnviron) terminateInstances(ids ...instance.Id) error {
	wg := sync.WaitGroup{}
	errc := make(chan error, len(ids))
	wg.Add(len(ids))
	for _, id := range ids {
		vmId := id
		go func() {
			defer wg.Done()
			if err := o.p.client.DeleteInstance(string(vmId)); err != nil {
				if !oci.IsNotFound(err) {
					errc <- err
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return errors.Annotate(err, "cannot stop all instances")
	default:
	}
	return nil
}

type tagValue struct {
	tag, value string
}

func (t *tagValue) String() string {
	return fmt.Sprintf("%s=%s", t.tag, t.value)
}

// allControllerManagedInstances returns all instances managed by this
// environment's controller, matching the optionally specified filter.
func (o *oracleEnviron) allControllerManagedInstances(controllerUUID string) ([]instance.Instance, error) {
	tagFilter := tagValue{tags.JujuController, controllerUUID}
	return o.allInstances(tagFilter)
}

func (o *oracleEnviron) allInstances(tagFilter tagValue) ([]instance.Instance, error) {
	resp, err := o.p.client.AllInstances()
	if err != nil {
		return nil, err
	}

	if len(resp.Result) == 0 {
		return nil, environs.ErrNoInstances
	}
	instances := []instance.Instance{}
	for _, val := range resp.Result {
		found := false
		for _, tag := range val.Tags {
			if tagFilter.String() == tag {
				found = true
				break
			}
		}
		if found == false {
			continue

		}
		oracleInstance, err := newInstance(&val, o.p.client)
		if err != nil {
			return nil, errors.Trace(err)
		}
		instances = append(instances, oracleInstance)
	}
	return instances, nil
}

// AllInstances returns all instances currently known to the broker.
func (o *oracleEnviron) AllInstances() ([]instance.Instance, error) {
	tagFilter := tagValue{tags.JujuModel, o.Config().UUID()}
	return o.allInstances(tagFilter)
}

// MaintainInstance is used to run actions on jujud startup for existing
// instances. It is currently only used to ensure that LXC hosts have the
// correct network configuration.
func (o *oracleEnviron) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// Config returns the configuration data with which the Environ was created.
// Note that this is not necessarily current; the canonical location
// for the configuration data is stored in the state.
func (o *oracleEnviron) Config() *config.Config {
	o.mutex.Lock()
	defer o.mutex.Unlock()
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
func (o *oracleEnviron) ConstraintsValidator() (constraints.Validator, error) {
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
	logger.Infof("Returning constraints validator: %v", validator)
	return validator, nil
}

// SetConfig updates the Environ's configuration.
//
// Calls to SetConfig do not affect the configuration of
// values previously obtained from Storage.
func (o *oracleEnviron) SetConfig(cfg *config.Config) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

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
func (o *oracleEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	instances := make([]instance.Instance, len(ids))
	all, err := o.AllInstances()
	if err != nil {
		return nil, errors.Trace(err)
	}
	found := 0

	for i, id := range ids {
		for _, inst := range all {
			if inst.Id() == id {
				instances[i] = inst
				found++
			}
		}
	}
	if found == 0 {
		return nil, environs.ErrNoInstances
	}

	if found != len(ids) {
		return instances, environs.ErrPartialInstances
	}
	return instances, nil
}

// ControllerInstances returns the IDs of instances corresponding
// to Juju controller, having the specified controller UUID.
// If there are no controller instances, ErrNoInstances is returned.
// If it can be determined that the environment has not been bootstrapped,
// then ErrNotBootstrapped should be returned instead.
func (o *oracleEnviron) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	instances, err := o.allControllerManagedInstances(controllerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filter := tagValue{tags.JujuIsController, "true"}
	ids := make([]instance.Id, 0, 1)
	for _, val := range instances {
		oracleInst := val.(*oracleInstance)
		found := false
		for _, tag := range oracleInst.machine.Tags {
			if tag == filter.String() {
				found = true
				break
			}
		}
		if found == false {
			continue
		}
		ids = append(ids, val.Id())
	}
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}
	return ids, nil
}

// Destroy shuts down all known machines and destroys the
// rest of the environment. Note that on some providers,
// very recently started instances may not be destroyed
// because they are not yet visible.
//
// When Destroy has been called, any Environ referring to the
// same remote environment may become invalid.
func (o *oracleEnviron) Destroy() error {
	return common.Destroy(o)
}

// DestroyController is similar to Destroy() in that it destroys
// the model, which in this case will be the controller model.
//
// In addition, this method also destroys any resources relating
// to hosted models on the controller on which it is invoked.
// This ensures that "kill-controller" can clean up hosted models
// when the Juju controller process is unavailable.
func (o *oracleEnviron) DestroyController(controllerUUID string) error {
	err := o.Destroy()
	if err != nil {
		logger.Errorf("Failed to destroy environment through controller")
	}

	instances, err := o.allControllerManagedInstances(controllerUUID)
	if err != nil {
		if err == environs.ErrNoInstances {
			return nil
		}
		return errors.Trace(err)
	}
	instIds := make([]instance.Id, len(instances))
	for i, val := range instances {
		instIds[i] = val.Id()
	}
	errc := make(chan error, len(instances))
	wg := sync.WaitGroup{}
	wg.Add(len(instances))
	for _, val := range instIds {
		go func() {
			defer wg.Done()
			err := o.terminateInstances(val)
			if !oci.IsNotFound(err) {
				errc <- err
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return errors.Annotate(err, "cannot stop all instances")
	default:
	}
	return nil
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (o *oracleEnviron) OpenPorts(rules []network.IngressRule) error {
	return nil
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (o *oracleEnviron) ClosePorts(rules []network.IngressRule) error {
	return nil
}

// IngressRules returns the ingress rules applied to the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
// It is expected that there be only one ingress rule result for a given
// port range - the rule's SourceCIDRs will contain all applicable source
// address rules for that port range.
func (o *oracleEnviron) IngressRules() ([]network.IngressRule, error) {
	return nil, nil
}

// Provider returns the EnvironProvider that created this Environ.
func (o *oracleEnviron) Provider() environs.EnvironProvider {
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
func (o *oracleEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return nil
}

// InstanceTypes allows for instance information from a provider to be obtained.
func (o *oracleEnviron) InstanceTypes(constraints.Value) (envinstance.InstanceTypesWithCostMetadata, error) {
	var i envinstance.InstanceTypesWithCostMetadata
	return i, nil
}
