// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	ociCommon "github.com/oracle/oci-go-sdk/v65/common"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	ociIdentity "github.com/oracle/oci-go-sdk/v65/identity"

	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/tools"
)

type Environ struct {
	common.CredentialInvalidator
	environs.NoSpaceDiscoveryEnviron
	environs.NoContainerAddressesEnviron

	Compute    ComputeClient
	Networking NetworkingClient
	Storage    StorageClient
	Firewall   FirewallClient
	Identity   IdentityClient
	ociConfig  ociCommon.ConfigurationProvider
	p          *EnvironProvider
	clock      clock.Clock
	ecfgMutex  sync.Mutex
	ecfgObj    *environConfig
	namespace  instance.Namespace

	// subnets contains one subnet for each availability domain
	// these will get created once the environment is spun up, and
	// will never change.
	subnets map[string][]ociCore.Subnet
}

var _ common.ZonedEnviron = (*Environ)(nil)
var _ storage.ProviderRegistry = (*Environ)(nil)
var _ environs.Environ = (*Environ)(nil)
var _ environs.Networking = (*Environ)(nil)
var _ environs.NetworkingEnviron = (*Environ)(nil)

func (e *Environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	return e.ecfgObj
}

func (e *Environ) allInstances(ctx context.Context, tags map[string]string) ([]*ociInstance, error) {
	compartment := e.ecfg().compartmentID()

	insts, err := e.Compute.ListInstances(context.Background(), compartment)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	ret := []*ociInstance{}
	for _, val := range insts {
		if val.LifecycleState == ociCore.InstanceLifecycleStateTerminated {
			continue
		}
		missingTag := false
		for i, j := range tags {
			tagVal, ok := val.FreeformTags[i]
			if !ok || tagVal != j {
				missingTag = true
				break
			}
		}
		if missingTag {
			// One of the tags was not found
			continue
		}
		inst, err := newInstance(val, e)
		if err != nil {
			return nil, e.HandleCredentialError(ctx, err)
		}
		ret = append(ret, inst)
	}
	return ret, nil
}

func (e *Environ) getOCIInstance(ctx envcontext.ProviderCallContext, id instance.Id) (*ociInstance, error) {
	instanceId := string(id)
	request := ociCore.GetInstanceRequest{
		InstanceId: &instanceId,
	}

	response, err := e.Compute.GetInstance(context.Background(), request)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	return newInstance(response.Instance, e)
}

func (e *Environ) isNotFound(response *http.Response) bool {
	if response.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

// waitForResourceStatus will ping the resource until the fetch function returns true,
// the timeout is reached, or an error occurs.
func (e *Environ) waitForResourceStatus(
	statusFunc func(resID *string) (status string, err error),
	resId *string, desiredStatus string,
	timeout time.Duration,
) error {

	var status string
	var err error
	timeoutTimer := e.clock.NewTimer(timeout)
	defer timeoutTimer.Stop()

	retryTimer := e.clock.NewTimer(0)
	defer retryTimer.Stop()

	for {
		select {
		case <-retryTimer.Chan():
			status, err = statusFunc(resId)
			if err != nil {
				return err
			}
			if status == desiredStatus {
				return nil
			}
			retryTimer.Reset(2 * time.Second)
		case <-timeoutTimer.Chan():
			return errors.Errorf(
				"timed out waiting for resource %q to transition to %v. Current status: %q",
				*resId, desiredStatus, status,
			)
		}
	}
}

func (e *Environ) ping() error {
	request := ociIdentity.ListAvailabilityDomainsRequest{
		CompartmentId: e.ecfg().compartmentID(),
	}
	_, err := e.Identity.ListAvailabilityDomains(context.Background(), request)
	return err
}

// AvailabilityZones is defined in the common.ZonedEnviron interface
func (e *Environ) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	request := ociIdentity.ListAvailabilityDomainsRequest{
		CompartmentId: e.ecfg().compartmentID(),
	}

	ociCtx := context.Background()
	domains, err := e.Identity.ListAvailabilityDomains(ociCtx, request)

	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	zones := network.AvailabilityZones{}

	for _, val := range domains.Items {
		zones = append(zones, NewAvailabilityZone(*val.Name))
	}
	return zones, nil
}

// InstanceAvailabilityZoneNames implements common.ZonedEnviron.
func (e *Environ) InstanceAvailabilityZoneNames(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
	instances, err := e.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, e.HandleCredentialError(ctx, err)
	}
	zones := make(map[instance.Id]string, 0)
	for _, inst := range instances {
		oInst, ok := inst.(*ociInstance)
		if !ok {
			continue
		}
		zones[inst.Id()] = oInst.availabilityZone()
	}
	if len(zones) < len(ids) {
		return zones, environs.ErrPartialInstances
	}
	return zones, nil
}

// DeriveAvailabilityZones implements common.ZonedEnviron.
func (e *Environ) DeriveAvailabilityZones(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	return nil, nil
}

func (e *Environ) getOciInstances(ctx context.Context, ids ...instance.Id) ([]*ociInstance, error) {
	ret := []*ociInstance{}

	compartmentID := e.ecfg().compartmentID()

	instances, err := e.Compute.ListInstances(context.Background(), compartmentID)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	if len(instances) == 0 {
		return nil, environs.ErrNoInstances
	}

	for _, val := range instances {
		oInstance, err := newInstance(val, e)
		if err != nil {
			return nil, e.HandleCredentialError(ctx, err)
		}
		for _, id := range ids {
			if oInstance.Id() == id {
				ret = append(ret, oInstance)
			}
		}
	}

	if len(ret) < len(ids) {
		return ret, environs.ErrPartialInstances
	}
	return ret, nil
}

func (e *Environ) getOciInstancesAsMap(ctx envcontext.ProviderCallContext, ids ...instance.Id) (map[instance.Id]*ociInstance, error) {
	instances, err := e.getOciInstances(ctx, ids...)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	ret := map[instance.Id]*ociInstance{}
	for _, inst := range instances {
		ret[inst.Id()] = inst
	}
	return ret, nil
}

// Instances implements environs.Environ.
func (e *Environ) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ociInstances, err := e.getOciInstances(ctx, ids...)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, e.HandleCredentialError(ctx, err)
	}

	ret := []instances.Instance{}
	for _, val := range ociInstances {
		ret = append(ret, val)
	}

	if len(ret) < len(ids) {
		return ret, environs.ErrPartialInstances
	}
	return ret, nil
}

// PrepareForBootstrap implements environs.Environ.
func (e *Environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	if ctx.ShouldVerifyCredentials() {
		logger.Infof(ctx, "Logging into the oracle cloud infrastructure")
		if err := e.ping(); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// Bootstrap implements environs.Environ.
func (e *Environ) Bootstrap(ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, callCtx, params)
}

// Create implements environs.Environ.
func (e *Environ) Create(ctx envcontext.ProviderCallContext, params environs.CreateParams) error {
	if err := e.ping(); err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	return nil
}

// AdoptResources implements environs.Environ.
func (e *Environ) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	// TODO(cderici): implement AdoptResources for oci
	return errors.NotImplementedf("AdoptResources")
}

// list of unsupported OCI provider constraints
var unsupportedConstraints = []string{
	constraints.Container,
	constraints.VirtType,
	constraints.Tags,
	constraints.ImageID,
}

// ConstraintsValidator implements environs.Environ.
func (e *Environ) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{corearch.AMD64, corearch.ARM64})
	logger.Infof(ctx, "Returning constraints validator: %v", validator)
	return validator, nil
}

// SetConfig implements environs.Environ.
func (e *Environ) SetConfig(ctx context.Context, cfg *config.Config) error {
	ecfg, err := e.p.newConfig(ctx, cfg)
	if err != nil {
		return err
	}

	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.ecfgObj = ecfg

	return nil
}

func (e *Environ) allControllerManagedInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]*ociInstance, error) {
	tags := map[string]string{
		tags.JujuController: controllerUUID,
	}
	return e.allInstances(ctx, tags)
}

// ControllerInstances implements environs.Environ.
func (e *Environ) ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	tags := map[string]string{
		tags.JujuController:   controllerUUID,
		tags.JujuIsController: "true",
	}
	instances, err := e.allInstances(ctx, tags)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	ids := []instance.Id{}
	for _, val := range instances {
		ids = append(ids, val.Id())
	}
	return ids, nil
}

// Destroy implements environs.Environ.
func (e *Environ) Destroy(ctx envcontext.ProviderCallContext) error {
	return common.Destroy(e, ctx)
}

// DestroyController implements environs.Environ.
func (e *Environ) DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error {
	err := e.Destroy(ctx)
	if err != nil {
		err = e.HandleCredentialError(ctx, err)
		logger.Errorf(ctx, "Failed to destroy environment through controller: %s", errors.Trace(err))
	}
	instances, err := e.allControllerManagedInstances(ctx, controllerUUID)
	if err != nil {
		if err == environs.ErrNoInstances {
			return nil
		}
		return e.HandleCredentialError(ctx, err)
	}
	ids := make([]instance.Id, len(instances))
	for i, val := range instances {
		ids[i] = val.Id()
	}

	err = e.StopInstances(ctx, ids...)
	if err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	logger.Debugf(ctx, "Cleaning up network resources")
	err = e.cleanupNetworksAndSubnets(ctx, controllerUUID, "")
	if err != nil {
		return e.HandleCredentialError(ctx, err)
	}

	return nil
}

// Provider implements environs.Environ.
func (e *Environ) Provider() environs.EnvironProvider {
	return e.p
}

// getCloudInitConfig returns a CloudConfig instance. The default oracle images come
// bundled with iptables-persistent on Ubuntu and firewalld on CentOS, which maintains
// a number of iptables firewall rules. We need to at least allow the juju API port for state
// machines. SSH port is allowed by default on linux images.
func (e *Environ) getCloudInitConfig(osname string, apiPort int, statePort int) (cloudinit.CloudConfig, error) {
	// TODO (gsamfira): remove this function when the above mention bug is fixed
	cloudcfg, err := cloudinit.New(osname)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}

	if apiPort == 0 || statePort == 0 {
		return cloudcfg, nil
	}

	operatingSystem := ostype.OSTypeForName(osname)
	switch operatingSystem {
	case ostype.Ubuntu:
		cloudcfg.AddRunCmd(fmt.Sprintf("/sbin/iptables -I INPUT -p tcp --dport %d -j ACCEPT", apiPort))
		cloudcfg.AddRunCmd(fmt.Sprintf("/sbin/iptables -I INPUT -p tcp --dport %d -j ACCEPT", statePort))
		cloudcfg.AddScripts("/etc/init.d/netfilter-persistent save")
	}
	return cloudcfg, nil
}

// StartInstance implements environs.InstanceBroker.
func (e *Environ) StartInstance(
	ctx envcontext.ProviderCallContext, args environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {
	result, err := e.startInstance(ctx, args)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	return result, nil
}

func (e *Environ) startInstance(
	ctx envcontext.ProviderCallContext, args environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {
	if args.ControllerUUID == "" {
		return nil, errors.NotFoundf("Controller UUID")
	}

	networks, err := e.ensureNetworksAndSubnets(ctx, args.ControllerUUID, e.Config().UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	zones, err := e.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	zone := zones[0].Name()
	network := networks[zone][0]
	// refresh the global image cache
	// this only hits the API every 30 minutes, otherwise just retrieves
	// from cache
	imgCache, err := refreshImageCache(ctx, e.Compute, e.ecfg().compartmentID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if logger.IsLevelEnabled(corelogger.TRACE) {
		logger.Tracef(ctx, "Image cache contains: %# v", pretty.Formatter(imgCache))
	}

	arch, err := args.Tools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}

	defaultType := VirtualMachine.String()
	if args.Constraints.VirtType == nil {
		args.Constraints.VirtType = &defaultType
	}

	// check if we find an image that is compliant with the
	// constraints provided in the oracle cloud account
	spec, image, err := findInstanceSpec(
		ctx,
		args.InstanceConfig.Base,
		arch,
		args.Constraints,
		imgCache,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	tools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef(ctx, "agent binaries: %v", tools)
	if err = args.InstanceConfig.SetTools(tools); err != nil {
		return nil, errors.Trace(err)
	}

	if err = instancecfg.FinishInstanceConfig(args.InstanceConfig, e.Config()); err != nil {
		return nil, errors.Trace(err)
	}
	hostname, err := e.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := args.InstanceConfig.Tags

	var apiPort int
	var statePort int
	var desiredStatus ociCore.InstanceLifecycleStateEnum
	// If we are bootstrapping a new controller, we want to wait for the
	// machine to reach running state before attempting to SSH into it,
	// to configure the controller.
	// If the machine that is spawning is not a controller, then userdata
	// will take care of it's initial setup, and waiting for a running
	// status is not necessary
	if args.InstanceConfig.IsController() {
		apiPort = args.InstanceConfig.ControllerConfig.APIPort()
		statePort = args.InstanceConfig.ControllerConfig.StatePort()
		desiredStatus = ociCore.InstanceLifecycleStateRunning
	} else {
		desiredStatus = ociCore.InstanceLifecycleStateProvisioning
	}

	cloudcfg, err := e.getCloudInitConfig(args.InstanceConfig.Base.OS, apiPort, statePort)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}

	// compose userdata with the cloud config template
	logger.Debugf(ctx, "Composing userdata")
	userData, err := providerinit.ComposeUserData(
		args.InstanceConfig,
		cloudcfg,
		OCIRenderer{},
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}

	var rootDiskSizeGB int64
	if args.Constraints.RootDisk != nil {
		rootDiskSizeGB = int64(*args.Constraints.RootDisk) / 1024
		if int(*args.Constraints.RootDisk) < MinVolumeSizeMB {
			logger.Warningf(ctx,
				"selected disk size is too small (%d MB). Setting root disk size to minimum volume size (%d MB)",
				int(*args.Constraints.RootDisk), MinVolumeSizeMB)
			rootDiskSizeGB = MinVolumeSizeMB / 1024
		} else if int(*args.Constraints.RootDisk) > MaxVolumeSizeMB {
			logger.Warningf(ctx,
				"selected disk size is too large (%d MB). Setting root disk size to maximum volume size (%d MB)",
				int(*args.Constraints.RootDisk), MaxVolumeSizeMB)
			rootDiskSizeGB = MaxVolumeSizeMB / 1024
		}
	} else {
		rootDiskSizeGB = MinVolumeSizeMB / 1024
	}

	allocatePublicIP := true
	if args.Constraints.HasAllocatePublicIP() {
		allocatePublicIP = *args.Constraints.AllocatePublicIP
	}

	bootSource := ociCore.InstanceSourceViaImageDetails{
		ImageId:             &image,
		BootVolumeSizeInGBs: &rootDiskSizeGB,
	}
	instanceDetails := ociCore.LaunchInstanceDetails{
		AvailabilityDomain: &zone,
		CompartmentId:      e.ecfg().compartmentID(),
		SourceDetails:      bootSource,
		Shape:              &spec.InstanceType.Name,
		CreateVnicDetails: &ociCore.CreateVnicDetails{
			SubnetId:       network.Id,
			AssignPublicIp: &allocatePublicIP,
			DisplayName:    &hostname,
		},
		DisplayName: &hostname,
		Metadata: map[string]string{
			"user_data": string(userData),
		},
		FreeformTags: tags,
	}

	ensureShapeConfig(spec.InstanceType, args.Constraints, &instanceDetails)

	request := ociCore.LaunchInstanceRequest{
		LaunchInstanceDetails: instanceDetails,
	}

	response, err := e.Compute.LaunchInstance(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instance, err := newInstance(response.Instance, e)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineId := response.Instance.Id
	timeout := 10 * time.Minute
	if err := instance.waitForMachineStatus(desiredStatus, timeout); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof(ctx, "started instance %q", *machineId)

	if desiredStatus == ociCore.InstanceLifecycleStateRunning && allocatePublicIP {
		if err := instance.waitForPublicIP(ctx); err != nil {
			return nil, errors.Trace(err)
		}
	}

	result := &environs.StartInstanceResult{
		DisplayName: hostname,
		Instance:    instance,
		Hardware:    instance.hardwareCharacteristics(),
	}

	return result, nil
}

func ensureShapeConfig(
	instanceSpec instances.InstanceType,
	constraints constraints.Value,
	instanceDetails *ociCore.LaunchInstanceDetails) {

	// If the selected spec is a flexible shape, we must provide the number
	// of OCPUs at least, so if the user hasn't provided cpu constraints we
	// must pass the default value.
	if (instanceSpec.MaxCpuCores != nil && instanceSpec.MaxCpuCores != &instanceSpec.CpuCores) ||
		(instanceSpec.MaxMem != nil && instanceSpec.MaxMem != &instanceSpec.Mem) {
		instanceDetails.ShapeConfig = &ociCore.LaunchInstanceShapeConfigDetails{}
		if constraints.HasCpuCores() {
			cpuCores := float32(*constraints.CpuCores)
			instanceDetails.ShapeConfig.Ocpus = &cpuCores
		} else {
			cpuCores := float32(instances.MinCpuCores)
			instanceDetails.ShapeConfig.Ocpus = &cpuCores
		}
		// If we don't set the memory on ShapeConfig, OCI uses a
		// default value of memory per Ocpu core. For example, for the
		// VM.Standard.A1.Flex, if we set 2 Ocpus OCI will set 12GB of
		// memory (default is 6GB per core).
		if constraints.HasMem() {
			mem := float32(*constraints.Mem / 1024)
			instanceDetails.ShapeConfig.MemoryInGBs = &mem
		}
	}
}

// StopInstances implements environs.InstanceBroker.
func (e *Environ) StopInstances(ctx envcontext.ProviderCallContext, ids ...instance.Id) error {
	ociInstances, err := e.getOciInstances(ctx, ids...)
	if err == environs.ErrNoInstances {
		return nil
	} else if err != nil {
		return e.HandleCredentialError(ctx, err)
	}

	logger.Debugf(ctx, "terminating instances %v", ids)
	if err := e.terminateInstances(ctx, ociInstances...); err != nil {
		return e.HandleCredentialError(ctx, err)
	}

	return nil
}

type instError struct {
	id  instance.Id
	err error
}

func (e *Environ) terminateInstances(ctx envcontext.ProviderCallContext, instances ...*ociInstance) error {
	wg := sync.WaitGroup{}
	wg.Add(len(instances))
	errCh := make(chan instError, len(instances))
	for _, oInst := range instances {
		go func(inst *ociInstance) {
			defer wg.Done()
			if err := inst.deleteInstance(ctx); err != nil {
				errCh <- instError{id: inst.Id(), err: err}
				_ = e.HandleCredentialError(ctx, err)
				return
			}
			err := inst.waitForMachineStatus(
				ociCore.InstanceLifecycleStateTerminated,
				resourcePollTimeout)
			if err != nil && !errors.Is(err, errors.NotFound) {
				err = e.HandleCredentialError(ctx, err)
				errCh <- instError{id: inst.Id(), err: err}
			}
		}(oInst)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	var instIds []instance.Id
	for item := range errCh {
		errs = append(errs, item.err)
		instIds = append(instIds, item.id)
	}

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

// AllInstances implements environs.InstanceBroker.
func (e *Environ) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	tags := map[string]string{
		tags.JujuModel: e.Config().UUID(),
	}
	allInstances, err := e.allInstances(ctx, tags)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	ret := []instances.Instance{}
	for _, val := range allInstances {
		ret = append(ret, val)
	}
	return ret, nil
}

// AllRunningInstances implements environs.InstanceBroker.
func (e *Environ) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	// e.allInstances() returns all but 'terminated' instances already, so
	// "all instances is the same as "all running" instances here.
	return e.AllInstances(ctx)
}

// Config implements environs.ConfigGetter.
func (e *Environ) Config() *config.Config {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	if e.ecfgObj == nil {
		return nil
	}
	return e.ecfgObj.Config
}

// PrecheckInstance implements environs.InstancePrechecker.
func (e *Environ) PrecheckInstance(envcontext.ProviderCallContext, environs.PrecheckInstanceParams) error {
	return nil
}

// InstanceTypes implements environs.InstancePrechecker.
func (e *Environ) InstanceTypes(context.Context, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, errors.NotImplementedf("InstanceTypes")
}
