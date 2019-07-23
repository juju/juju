// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	oci "github.com/juju/go-oracle-cloud/api"
	ociCommon "github.com/juju/go-oracle-cloud/common"
	ociResponse "github.com/juju/go-oracle-cloud/response"
	"github.com/juju/os"
	jujuseries "github.com/juju/os/series"
	"github.com/juju/utils/arch"
	"github.com/juju/version"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	envinstance "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	commonProvider "github.com/juju/juju/provider/oracle/common"
	oraclenet "github.com/juju/juju/provider/oracle/network"
	"github.com/juju/juju/tools"
)

// OracleEnviron implements the environs.Environ interface
type OracleEnviron struct {
	environs.Networking
	oraclenet.Firewaller

	mutex     *sync.Mutex
	p         *EnvironProvider
	spec      environs.CloudSpec
	cfg       *config.Config
	client    EnvironAPI
	namespace instance.Namespace
	clock     clock.Clock
	rand      *rand.Rand
}

// EnvironAPI provides interface to access and make operation
// inside a oracle environ
type EnvironAPI interface {
	commonProvider.Instancer
	commonProvider.InstanceAPI
	commonProvider.Authenticater
	commonProvider.Shaper
	commonProvider.Imager
	commonProvider.IpReservationAPI
	commonProvider.IpAssociationAPI
	commonProvider.IpNetworkExchanger
	commonProvider.IpNetworker
	commonProvider.VnicSetAPI

	commonProvider.RulesAPI
	commonProvider.AclAPI
	commonProvider.SecIpAPI
	commonProvider.IpAddressPrefixSetAPI
	commonProvider.SecListAPI
	commonProvider.ApplicationsAPI
	commonProvider.SecRulesAPI
	commonProvider.AssociationAPI

	StorageAPI
}

// AvailabilityZones is defined in the common.ZonedEnviron interface
func (o *OracleEnviron) AvailabilityZones(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
	return []common.AvailabilityZone{
		oraclenet.NewAvailabilityZone("default"),
	}, nil
}

// InstanceAvailabilityzoneNames is defined in the common.ZonedEnviron interface
func (o *OracleEnviron) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
	instances, err := o.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}
	zones := make([]string, len(instances))
	for idx := range instances {
		zones[idx] = "default"
	}
	return zones, nil
}

// NewOracleEnviron returns a new OracleEnviron
func NewOracleEnviron(p *EnvironProvider, args environs.OpenParams, client EnvironAPI, c clock.Clock) (env *OracleEnviron, err error) {
	if client == nil {
		return nil, errors.NotFoundf("oracle client")
	}
	if p == nil {
		return nil, errors.NotFoundf("environ proivder")
	}
	env = &OracleEnviron{
		p:      p,
		spec:   args.Cloud,
		cfg:    args.Config,
		mutex:  &sync.Mutex{},
		client: client,
		clock:  c,
	}
	env.namespace, err = instance.NewNamespace(env.cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	env.Firewaller = oraclenet.NewFirewall(env, client, c)
	env.Networking = oraclenet.NewEnviron(client, env)

	source := rand.NewSource(env.clock.Now().UTC().UnixNano())
	r := rand.New(source)
	env.rand = r

	return env, nil
}

// PrepareForBootstrap is part of the Environ interface.
func (o *OracleEnviron) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	if ctx.ShouldVerifyCredentials() {
		logger.Infof("Logging into the oracle cloud infrastructure")
		if err := o.client.Authenticate(); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// Bootstrap is part of the Environ interface.
func (o *OracleEnviron) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, o, callCtx, args)
}

// Create is part of the Environ interface.
func (o *OracleEnviron) Create(ctx context.ProviderCallContext, params environs.CreateParams) error {
	if err := o.client.Authenticate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// AdoptResources is part of the Environ interface.
func (e *OracleEnviron) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	//TODO (gsamfira): Implement AdoptResources. Controller tag for all resources needs to
	// be changed
	return nil
}

// getCloudInitConfig returns a CloudConfig instance. this function only exists because of a
// limitation that currently exists in cloud-init see bug report:
// https://bugs.launchpad.net/cloud-init/+bug/1675370
func (e *OracleEnviron) getCloudInitConfig(series string, networks map[string]oci.Networker) (cloudinit.CloudConfig, error) {
	// TODO (gsamfira): remove this function when the above mention bug is fixed
	cloudcfg, err := cloudinit.New(series)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}
	operatingSystem, err := jujuseries.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	renderer := cloudcfg.ShellRenderer()
	// by default windows has all NICs set on DHCP, so no need to configure
	// anything.
	// gsamfira: I could not find a CentOS image in the oracle compute
	// marketplace, so not doing anything CentOS specific here.
	var scripts []string
	switch operatingSystem {
	case os.Ubuntu:
		for key := range networks {
			if key == defaultNicName {
				continue
			}
			fileBaseName := fmt.Sprintf("%s.cfg", key)
			fileContents := fmt.Sprintf(ubuntuInterfaceTemplate, key, key)
			fileName := renderer.Join(
				renderer.FromSlash(interfacesConfigDir), fileBaseName)
			scripts = append(scripts,
				renderer.WriteFile(fileName, []byte(fileContents))...)
			scripts = append(scripts, fmt.Sprintf("/sbin/ifup %s", key))
			scripts = append(scripts, "")

		}
		if len(scripts) > 0 {
			cloudcfg.AddScripts(strings.Join(scripts, "\n"))
		}
	}
	return cloudcfg, nil
}

// buildSpacesMap builds a map with juju converted names from provider space names
//
// shamelessly copied from the MAAS provider
func (e *OracleEnviron) buildSpacesMap(
	ctx context.ProviderCallContext,
) (map[string]corenetwork.SpaceInfo, map[string]string, error) {
	empty := set.Strings{}
	providerIdMap := map[string]string{}
	// NOTE (gsamfira): This seems brittle to me, and I would much rather get this
	// from state, as that information should already be there from the discovered spaces
	// and that is the information that gets presented to the user when running:
	// juju spaces
	// However I have not found a clean way to access that info from the provider,
	// without creating a facade. Someone with more knowledge on this might be able to chip in.
	spaces, err := e.Spaces(ctx)
	if err != nil {
		return nil, providerIdMap, errors.Trace(err)
	}
	spaceMap := make(map[string]corenetwork.SpaceInfo)
	for _, space := range spaces {
		jujuName := network.ConvertSpaceName(space.Name, empty)
		spaceMap[jujuName] = space
		empty.Add(jujuName)
		providerIdMap[string(space.ProviderId)] = space.Name
	}
	return spaceMap, providerIdMap, nil

}

func (e *OracleEnviron) getInstanceNetworks(
	ctx context.ProviderCallContext,
	args environs.StartInstanceParams,
	secLists, vnicSets []string,
) (map[string]oci.Networker, error) {

	// gsamfira: We add a default NIC attached o the shared network provided
	// by the oracle cloud. This NIC is used for outbound traffic.
	// While you can attach just a VNIC to the instance, and assign a
	// public IP to it, I have not been able to get any outbound traffic
	// through the thing.
	// NOTE (gsamfira): The NIC ordering inside the instance is determined
	// by the order in which the API returns the network interfaces. It seems
	// the API orders the NICs alphanumerically. When adding new network
	// interfaces, make sure they are ordered to come after eth0
	networking := map[string]oci.Networker{
		defaultNicName: oci.SharedNetwork{
			Seclists: secLists,
		},
	}
	spaces := set.Strings{}
	if s := args.Constraints.IncludeSpaces(); len(s) != 0 {
		for _, val := range s {
			logger.Debugf("Adding space from constraints %s", val)
			spaces.Add(val)
		}
	}

	// NOTE (gsamfira): The way spaces works seems really iffy to me. We currently
	// rely on two sources of truth to determine the spaces to which we should attach
	// an instance. This becomes evident then we try to use --constraints and --bind
	// to specify the space.
	// In both cases, the user specifies the space name that comes up in juju spaces
	// When fetching the space using constraints inside the provider, we get the actual
	// space name passed in by the user. However, when we go through args.EndpointBindings
	// we get a mapping between the endpoint binding name (not the space name) and the ProviderID
	// of the space (unlike constraints where you get the name). All this without being able to access
	// the source of truth the user used when selecting the space, which is juju state. So we need to:
	// 1) fetch spaces from the provider
	// 2) parse the name and mutate it to match what the discover spaces worker does (and hope
	// that the API returns the spaces in the same order every time)
	// 3) create a map of those spaces both name-->space and providerID-->name to be able to match
	// both cases. This all seems really brittle to me.
	providerSpaces, providerIds, err := e.buildSpacesMap(ctx)
	if err != nil {
		return map[string]oci.Networker{}, err
	}

	if len(args.EndpointBindings) != 0 {
		for _, providerID := range args.EndpointBindings {
			if name, ok := providerIds[string(providerID)]; ok {
				logger.Debugf("Adding space from bindings %s", name)
				spaces.Add(name)
			}
		}
	}

	//start from 1. eth0 is the default nic that gets attached by default.
	idx := 1
	for _, space := range spaces.Values() {
		providerSpace, ok := providerSpaces[space]
		if !ok {
			return map[string]oci.Networker{},
				errors.Errorf("Could not find space %q", space)
		}
		if len(providerSpace.Subnets) == 0 {
			return map[string]oci.Networker{},
				errors.Errorf("No usable subnets found in space %q", space)
		}

		ipNet := providerSpace.Subnets[e.rand.Intn(len(providerSpace.Subnets))]
		vnic := oci.IPNetwork{
			Ipnetwork: string(ipNet.ProviderId),
			Vnicsets:  vnicSets,
		}
		nicName := fmt.Sprintf("%s%s", nicPrefix, strconv.Itoa(idx))
		networking[nicName] = vnic
		idx++
	}
	logger.Debugf("returning networking interfaces: %v", networking)
	return networking, nil
}

// StartInstance is part of the InstanceBroker interface.
func (o *OracleEnviron) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
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
	if args.ImageMetadata, err = checkImageList(o.client); err != nil {
		return nil, errors.Trace(err)
	}

	// find the best suitable instance based on
	// the oracle cloud instance types,
	// the images that already matched the juju constrains
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
	logger.Tracef("agent binaries: %v", tools)
	if err = args.InstanceConfig.SetTools(tools); err != nil {
		return nil, errors.Trace(err)
	}

	if err = instancecfg.FinishInstanceConfig(
		args.InstanceConfig,
		o.Config(),
	); err != nil {
		return nil, errors.Trace(err)
	}

	hostname, err := o.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, errors.Trace(err)
	}

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
	var desiredStatus ociCommon.InstanceState
	if args.InstanceConfig.Controller != nil {
		apiPort = args.InstanceConfig.Controller.Config.APIPort()
		desiredStatus = ociCommon.StateRunning
	} else {
		// All ports are the same so pick the first.
		apiPort = args.InstanceConfig.APIInfo.Ports()[0]
		desiredStatus = ociCommon.StateStarting
	}

	// create a new seclists
	secLists, err := o.CreateMachineSecLists(
		args.InstanceConfig.MachineId, apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("Creating vnic sets")
	vnicSets, err := o.ensureVnicSet(ctx, args.InstanceConfig.MachineId, tags)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// fetch instance network card configuration
	logger.Debugf("Getting instance networks")
	networking, err := o.getInstanceNetworks(ctx, args, secLists, []string{vnicSets.Name})
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Debugf("Getting cloud config")
	cloudcfg, err := o.getCloudInitConfig(args.InstanceConfig.Series, networking)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}

	// compose userdata with the cloud config template
	logger.Debugf("Composing userdata")
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

	// create the instance based on the instance params
	instance, err := o.createInstance(oci.InstanceParams{
		Instances: []oci.Instances{
			{
				Shape:       spec.InstanceType.Name,
				Imagelist:   imageName,
				Name:        machineName,
				Label:       args.InstanceConfig.MachineAgentServiceName,
				Hostname:    hostname,
				Tags:        tags,
				Attributes:  attributes,
				Reverse_dns: true,
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
	if err := instance.waitForMachineStatus(desiredStatus, timeout); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("started instance %q", machineId)

	//TODO: add config option for public IP allocation
	logger.Debugf("Associating public IP to instance %q", machineId)
	if err := instance.associatePublicIP(); err != nil {
		return nil, errors.Trace(err)
	}
	result := &environs.StartInstanceResult{
		Instance: instance,
		Hardware: instance.hardwareCharacteristics(),
	}

	return result, nil
}

// StopInstances is part of the InstanceBroker interface.
func (o *OracleEnviron) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
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

func (o *OracleEnviron) terminateInstances(instances ...*oracleInstance) error {
	wg := sync.WaitGroup{}
	wg.Add(len(instances))
	errs := []error{}
	instIds := []instance.Id{}
	for _, oInst := range instances {
		inst := oInst
		go func() {
			defer wg.Done()
			if err := inst.deleteInstanceAndResources(true); err != nil {
				if !oci.IsNotFound(err) {
					instIds = append(instIds, instance.Id(inst.name))
					errs = append(errs, err)
				}
			}
		}()
	}
	wg.Wait()
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.Annotatef(errs[0], "failed to stop instance %s", instIds[0])
	default:
		return errors.Errorf(
			"failed to stop instances %s: %s",
			instIds, errs,
		)
	}
}

type tagValue struct {
	tag, value string
}

func (t *tagValue) String() string {
	return fmt.Sprintf("%s=%s", t.tag, t.value)
}

// allControllerManagedInstances returns all instances managed by this
// environment's controller, matching the optionally specified filter.
func (o *OracleEnviron) allControllerManagedInstances(controllerUUID string) ([]*oracleInstance, error) {
	return o.allInstances(tagValue{
		tag:   tags.JujuController,
		value: controllerUUID,
	})
}

// getOracleInstances attempts to fetch information from the oracle API for the
// specified IDs.
func (o *OracleEnviron) getOracleInstances(ids ...instance.Id) ([]*oracleInstance, error) {
	ret := make([]*oracleInstance, 0, len(ids))
	resp, err := o.client.AllInstances(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(resp.Result) == 0 {
		return nil, environs.ErrNoInstances
	}

	for _, val := range resp.Result {
		for _, id := range ids {
			oInst, err := newInstance(val, o)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if oInst.Id() == id {
				ret = append(ret, oInst)
				break
			}
		}
	}
	if len(ret) < len(ids) {
		return ret, environs.ErrPartialInstances
	}
	return ret, nil
}

func (o *OracleEnviron) getOracleInstancesAsMap(ids ...instance.Id) (map[string]*oracleInstance, error) {
	instances, err := o.getOracleInstances(ids...)
	if err != nil {
		return map[string]*oracleInstance{}, errors.Trace(err)
	}
	ret := map[string]*oracleInstance{}
	for _, val := range instances {
		ret[string(val.Id())] = val
	}
	return ret, nil
}

// AllInstances is part of the InstanceBroker interface.
func (o *OracleEnviron) AllInstances(ctx context.ProviderCallContext) ([]envinstance.Instance, error) {
	tagFilter := tagValue{tags.JujuModel, o.Config().UUID()}
	all, err := o.allInstances(tagFilter)
	if err != nil {
		return nil, err
	}

	ret := make([]envinstance.Instance, len(all))
	for i, val := range all {
		ret[i] = val
	}
	return ret, nil
}

// AllRunningInstances is part of the InstanceBroker interface.
func (o *OracleEnviron) AllRunningInstances(ctx context.ProviderCallContext) ([]envinstance.Instance, error) {
	// o.allInstances(...) already handles all instances irrespective of the state, so
	// here 'all' is also 'all running'.
	return o.AllInstances(ctx)
}

func (o *OracleEnviron) allInstances(tagFilter tagValue) ([]*oracleInstance, error) {
	filter := []oci.Filter{
		{
			Arg:   "tags",
			Value: tagFilter.String(),
		},
	}
	logger.Infof("Looking for instances with tags: %v", filter)
	resp, err := o.client.AllInstances(filter)
	if err != nil {
		return nil, err
	}

	n := len(resp.Result)
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

// MaintainInstance is part of the InstanceBroker interface.
func (o *OracleEnviron) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

// Config is part of the Environ interface.
func (o *OracleEnviron) Config() *config.Config {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	return o.cfg
}

// ConstraintsValidator is part of the environs.Environ interface.
func (o *OracleEnviron) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	// list of unsupported oracle provider constraints
	unsupportedConstraints := []string{
		constraints.Container,
		constraints.CpuPower,
		constraints.RootDisk,
		constraints.VirtType,
	}

	// we choose to use the default validator implementation
	validator := constraints.NewValidator()
	// we must feed the validator that the oracle cloud
	// provider does not support these constraints
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{arch.I386, arch.AMD64})
	logger.Infof("Returning constraints validator: %v", validator)
	return validator, nil
}

// SetConfig is part of the environs.Environ interface.
func (o *OracleEnviron) SetConfig(cfg *config.Config) error {
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

func (o *OracleEnviron) Details(id instance.Id) (ociResponse.Instance, error) {
	inst, err := o.getOracleInstances(id)
	if err != nil {
		return ociResponse.Instance{}, err
	}

	return inst[0].machine, nil
}

// Instances is part of the environs.Environ interface.
func (o *OracleEnviron) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]envinstance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	instances, err := o.getOracleInstances(ids...)
	if err != nil {
		return nil, err
	}

	ret := []envinstance.Instance{}
	for _, val := range instances {
		ret = append(ret, val)
	}
	return ret, nil
}

// ControllerInstances is part of the environs.Environ interface.
func (o *OracleEnviron) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
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

// Destroy is part of the environs.Environ interface.
func (o *OracleEnviron) Destroy(ctx context.ProviderCallContext) error {
	return common.Destroy(o, ctx)
}

// DestroyController is part of the environs.Environ interface.
func (o *OracleEnviron) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	err := o.Destroy(ctx)
	if err != nil {
		logger.Errorf("Failed to destroy environment through controller: %s", errors.Trace(err))
	}
	instances, err := o.allControllerManagedInstances(controllerUUID)
	if err != nil {
		if err == environs.ErrNoInstances {
			return nil
		}
		return errors.Trace(err)
	}
	ids := make([]instance.Id, len(instances))
	for i, val := range instances {
		ids[i] = val.Id()
	}
	return o.StopInstances(ctx, ids...)
}

// Provider is part of the environs.Environ interface.
func (o *OracleEnviron) Provider() environs.EnvironProvider {
	return o.p
}

// PrecheckInstance is part of the environs.Environ interface.
func (o *OracleEnviron) PrecheckInstance(context.ProviderCallContext, environs.PrecheckInstanceParams) error {
	return nil
}

// InstanceTypes is part of the environs.InstanceTypesFetcher interface.
func (o *OracleEnviron) InstanceTypes(context.ProviderCallContext, constraints.Value) (envinstance.InstanceTypesWithCostMetadata, error) {
	var i envinstance.InstanceTypesWithCostMetadata
	return i, nil
}

// createInstance creates a new instance inside the oracle infrastructure
func (e *OracleEnviron) createInstance(params oci.InstanceParams) (*oracleInstance, error) {
	if len(params.Instances) > 1 {
		return nil, errors.NotSupportedf("launching multiple instances")
	}

	logger.Debugf("running createInstance")
	resp, err := e.client.CreateInstance(params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instance, err := newInstance(resp.Instances[0], e)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return instance, nil
}
