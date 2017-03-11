// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	oci "github.com/juju/go-oracle-cloud/api"
	ociCommon "github.com/juju/go-oracle-cloud/common"
	"github.com/juju/version"

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
)

// oracleEnviron implements the environs.Environ interface
// and has behaviour specific that the interface provides.
type oracleEnviron struct {
	// mutex for synchronising stuff
	mutex *sync.Mutex

	// p is the internal envirnon provider
	p *environProvider
	// spec is the cloud spec of the provider
	spec environs.CloudSpec
	// cfg is the bootstrap config
	cfg *config.Config
	// firewall firewall type used in network operations
	firewall *Firewall
	// client is the internal api client with the
	// oralce cloud infrastructure
	client *oci.Client
	// namespace is the namespace of generated
	// for the current envirnonment
	namespace instance.Namespace
}

// newOracleEnviron returns a new oracleEnviron
// composed from a environProvider and args from environs.OpenParams,
// usually this method is used in Open method calls of the environProvider
func newOracleEnviron(
	p *environProvider,
	args environs.OpenParams,
	client *oci.Client,
) (*oracleEnviron, error) {

	if client == nil {
		return nil, errors.NotFoundf("oracle client")
	}

	if p == nil {
		return nil, errors.NotFoundf("environ proivder")
	}

	env := &oracleEnviron{
		p:      p,
		spec:   args.Cloud,
		cfg:    args.Config,
		mutex:  &sync.Mutex{},
		client: client,
	}
	// create a new firewall from the env and the internal api client
	env.firewall = NewFirewall(env, client)

	namespace, err := instance.NewNamespace(env.cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	env.namespace = namespace

	return env, nil
}

// PrepareForBootstrap prepares an environment for bootstrapping.
//
// This will be called very early in the bootstrap procedure, to
// give an Environ a chance to perform interactive operations that
// are required for bootstrapping.
func (o *oracleEnviron) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	if ctx.ShouldVerifyCredentials() {
		logger.Infof("Loging into the oracle cloud infrastructure")
		if err := o.client.Authenticate(); err != nil {
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
func (o *oracleEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {

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
	if err := o.client.Authenticate(); err != nil {
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
func (o *oracleEnviron) StartInstance(
	args environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {

	if args.ControllerUUID == "" {
		return nil, errors.NotFoundf("Controller UUID")
	}

	series := args.Tools.OneSeries()
	arches := args.Tools.Arches()

	// take all instance types from the oracle cloud provider
	types, err := instanceTypes(o.client)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// check if we find an image that is compliant with the
	// constraints provided in the oracle cloud account
	if args.ImageMetadata, err = checkImageList(
		o.client,
		args.Constraints,
	); err != nil {
		return nil, errors.Trace(err)
	}

	// find the best suitable instance based on
	// the oracle cloud instance types,
	// the images that already mached the juju constrains
	spec, imagelist, err := findInstanceSpec(
		o.client,
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

	if err = instancecfg.FinishInstanceConfig(
		args.InstanceConfig,
		o.Config(),
	); err != nil {
		return nil, errors.Trace(err)
	}

	// new cloud config template based on the series
	cloudcfg, err := cloudinit.New(args.InstanceConfig.Series)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}

	// compose userdata with the cloud config template
	userData, err := providerinit.ComposeUserData(
		args.InstanceConfig,
		cloudcfg,
		OracleRenderer{},
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}

	logger.Debugf("oracle user data: %d bytes", len(userData))

	attributes := map[string]interface{}{
		"userdata": string(userData),
	}

	hostname, err := o.namespace.Hostname(args.InstanceConfig.MachineId)
	machineName := o.client.ComposeName(hostname)
	imageName := o.client.ComposeName(imagelist)

	tags := make([]string, 0, len(args.InstanceConfig.Tags)+1)
	for k, v := range args.InstanceConfig.Tags {
		if k == "" || v == "" {
			continue
		}
		t := tagValue{k, v}
		tags = append(tags, t.String())
	}
	tags = append(tags, machineName)

	var apiPort int
	if args.InstanceConfig.Controller != nil {
		apiPort = args.InstanceConfig.Controller.Config.APIPort()
	} else {
		// All ports are the same so pick the first.
		apiPort = args.InstanceConfig.APIInfo.Ports()[0]
	}

	// create a new seclists
	secLists, err := o.firewall.CreateMachineSecLists(
		args.InstanceConfig.MachineId, apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// create a  new netowrking card used for making the instance
	// have a public address ip
	networking := map[string]oci.Networker{
		"eth0": oci.SharedNetwork{
			Seclists: secLists,
		},
	}

	// create the instance based on the instance params
	instance, err := o.createInstance(o.client, oci.InstanceParams{
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
				Networking:  networking,
			},
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	instance.arch = &spec.Image.Arch
	instance.instType = &spec.InstanceType

	machineId := instance.machine.Name
	timeout := 10 * time.Minute
	if err := instance.waitForMachineStatus(ociCommon.StateRunning, timeout); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("started instance %q", machineId)
	//TODO: add config option for public IP allocation
	logger.Infof("Associating public IP to instance %q", machineId)
	if err := instance.associatePublicIP(); err != nil {
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
	oracleInstances, err := o.getOracleInstances(ids...)
	if err == environs.ErrNoInstances {
		return nil
	} else if err != nil {
		return err
	}
	logger.Debugf("terminating instances %v", ids)
	if err := o.terminateInstances(oracleInstances...); err != nil {
		return err
	}
	return nil
}

func (o *oracleEnviron) terminateInstances(instances ...*oracleInstance) error {
	wg := sync.WaitGroup{}
	errc := make(chan error, len(instances))
	wg.Add(len(instances))
	for _, oInst := range instances {
		go func() {
			defer wg.Done()
			if err := oInst.delete(true); err != nil {
				if !oci.IsNotFound(err) {
					errc <- err
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return errors.Annotate(err, "cannot delete all instances")
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
func (o *oracleEnviron) allControllerManagedInstances(controllerUUID string) ([]*oracleInstance, error) {
	return o.allInstances(tagValue{
		tag:   tags.JujuController,
		value: controllerUUID,
	})
}

// getOracleInstances returns a slice of oracle instances given some instances ids
// if the given value of ids does not match the result of call this will return
// environ.ErrPartialInstances
// if we didn't found any instance that match any ids this will return
// environ.ErrNoInstances
func (o *oracleEnviron) getOracleInstances(ids ...instance.Id) ([]*oracleInstance, error) {
	ret := make([]*oracleInstance, 0, len(ids))

	// if the caller passed one instance
	if len(ids) == 1 {
		// get the instance
		inst, err := o.client.InstanceDetails(string(ids[0]))
		if err != nil {
			return nil, environs.ErrNoInstances
		}

		// parse the instance from the raw response
		oInst, err := newInstance(inst, o)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ret = append(ret, oInst)
		return ret, nil
	}

	// // we have more than one instance so we shall
	// get all of them
	resp, err := o.client.AllInstances(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(resp.Result) == 0 {
		return nil, environs.ErrNoInstances
	}

	nills := 0
	// for every instance we found
	for _, val := range resp.Result {
		found := false
		// for every ids provided
		for _, id := range ids {
			// if of the instance given match
			// the instances found in the infrastracture
			// mark the flag and break the loop
			if val.Name == string(id) {
				found = true
				break
			}
		}

		// continue if one of the
		// instance given is not found
		if found == false {
			ret = append(ret, nil)
			nills++
			continue
		}

		// parse the instance found
		oInst, err := newInstance(val, o)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// append the valid parsed one
		// into the result list
		ret = append(ret, oInst)
	}

	validOnes := len(ret) - nills
	if validOnes != len(ids) {
		return ret, environs.ErrPartialInstances
	}

	return ret, nil
}

// AllInstances returns all instances currently known to the broker
func (o *oracleEnviron) AllInstances() ([]instance.Instance, error) {
	tagFilter := tagValue{tags.JujuModel, o.Config().UUID()}
	instances, err := o.allInstances(tagFilter)
	if err != nil {
		return nil, err
	}

	ret := make([]instance.Instance, len(instances))
	for i, val := range instances {
		ret[i] = val
	}
	return ret, nil
}

// allInstances will return all oracle instances if we don't have any instance
// given the tag filter, this will return environs.ErrNoInstances
func (o *oracleEnviron) allInstances(tagFilter tagValue) ([]*oracleInstance, error) {
	filter := []oci.Filter{
		oci.Filter{
			Arg:   "tags",
			Value: tagFilter.String(),
		},
	}

	logger.Infof("Looking for instances with tags: %v", filter)

	resp, err := o.client.AllInstances(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	n := len(resp.Result)
	if n == 0 {
		return nil, environs.ErrNoInstances
	}

	instances := make([]*oracleInstance, 0, n)
	for _, val := range resp.Result {
		oracleInstance, err := newInstance(val, o)
		if err != nil {
			return nil, errors.Trace(err)
		}
		instances = append(instances, oracleInstance)
	}

	return instances, nil
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
// was no other error, it will return ErrNoInstances.
// If some but not all the instances were found,
// the returned slice will have some nil slots,
// and an ErrPartialInstances error will be returned.
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
		found := false
		for _, tag := range val.machine.Tags {
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
	errc := make(chan error, len(instances))
	wg := sync.WaitGroup{}
	wg.Add(len(instances))
	for _, val := range instances {
		go func() {
			defer wg.Done()
			err := val.delete(true)
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
	if o.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on model", o.Config().FirewallMode())
	}
	return o.firewall.OpenPorts(rules)
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (o *oracleEnviron) ClosePorts(rules []network.IngressRule) error {
	return o.firewall.ClosePorts(rules)
}

// IngressRules returns the ingress rules applied to the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
// It is expected that there be only one ingress rule result for a given
// port range - the rule's SourceCIDRs will contain all applicable source
// address rules for that port range.
func (o *oracleEnviron) IngressRules() ([]network.IngressRule, error) {
	return o.firewall.GlobalIngressRules()
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
