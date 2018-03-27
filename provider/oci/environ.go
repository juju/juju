// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	"github.com/juju/version"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	providerCommon "github.com/juju/juju/provider/oci/common"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"

	ociCore "github.com/oracle/oci-go-sdk/core"
	ociIdentity "github.com/oracle/oci-go-sdk/identity"
)

type Environ struct {
	// environs.Networking
	// environs.Firewaller

	cli providerCommon.ApiClient
	p   *EnvironProvider

	clock clock.Clock
	// cfg   *config.Config

	ecfgMutex sync.Mutex
	ecfgObj   *environConfig
	namespace instance.Namespace

	vcn     ociCore.Vcn
	seclist ociCore.SecurityList
	// subnets contains one subnet for each availability domain
	// these will get created once the environment is spun up, and
	// will never change.
	subnets map[string][]ociCore.Subnet
}

var (
	tcpProtocolNumber  = "6"
	udpProtocolNumber  = "17"
	icmpProtocolNumber = "1"
	allProtocols       = "all"
)

var _ common.ZonedEnviron = (*Environ)(nil)
var _ storage.ProviderRegistry = (*Environ)(nil)
var _ environs.Environ = (*Environ)(nil)
var _ environs.Firewaller = (*Environ)(nil)
var _ environs.Networking = (*Environ)(nil)
var _ environs.NetworkingEnviron = (*Environ)(nil)

// AvailabilityZones is defined in the common.ZonedEnviron interface
func (e *Environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	ocid, err := e.cli.TenancyOCID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	request := ociIdentity.ListAvailabilityDomainsRequest{
		CompartmentId: &ocid,
	}
	ctx := context.Background()
	domains, err := e.cli.ListAvailabilityDomains(ctx, request)
	if err != nil {
		return nil, errors.Trace(err)
	}

	zones := []common.AvailabilityZone{}

	for _, val := range domains.Items {
		zones = append(zones, NewAvailabilityZone(*val.Name))
	}
	return zones, nil
}

// InstanceAvailabilityzoneNames implements common.ZonedEnviron.
func (e *Environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	instances, err := e.Instances(ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}
	zones := make([]string, len(instances))
	for idx, _ := range instances {
		zones[idx] = "default"
	}
	return zones, nil
}

// DeriveAvailabilityZones implements common.ZonedEnviron.
func (e *Environ) DeriveAvailabilityZones(args environs.StartInstanceParams) ([]string, error) {
	return nil, nil
}

func (e *Environ) getOciInstances(ids ...instance.Id) ([]*ociInstance, error) {
	ret := []*ociInstance{}

	compartmentID := e.ecfg().compartmentID()
	request := ociCore.ListInstancesRequest{
		CompartmentId: compartmentID,
	}

	instances, err := e.cli.ListInstances(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(instances.Items) == 0 {
		return nil, environs.ErrNoInstances
	}

	for _, val := range instances.Items {
		oInstance, err := newInstance(val, e)
		if err != nil {
			return nil, errors.Trace(err)
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

func (e *Environ) getOciInstancesAsMap(ids ...instance.Id) (map[instance.Id]*ociInstance, error) {
	instances, err := e.getOciInstances(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ret := map[instance.Id]*ociInstance{}
	for _, inst := range instances {
		ret[inst.Id()] = inst
	}
	return ret, nil
}

// Instances implements environs.Environ.
func (e *Environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	instances, err := e.getOciInstances(ids...)
	if err != nil {
		return nil, err
	}

	ret := []instance.Instance{}
	for _, val := range instances {
		ret = append(ret, val)
	}
	return ret, nil
}

// PrepareForBootstrap implements environs.Environ.
func (e *Environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	if ctx.ShouldVerifyCredentials() {
		logger.Infof("Logging into the oracle cloud infrastructure")
		if err := e.cli.Ping(); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// Bootstrap implements environs.Environ.
func (e *Environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, params)
}

// TODO(gsamfira): Move the networking related bits out of this file

func (e *Environ) isNotFound(response *http.Response) bool {
	if response.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

// waitForResourceStatus will ping the resource until the fetch function returns true,
// the timeout is reached, or an error occurs.
func (o *Environ) waitForResourceStatus(
	statusFunc func(resID *string) (status string, err error),
	resId *string, desiredStatus string,
	timeout time.Duration,
) error {

	var status string
	var err error
	timeoutTimer := o.clock.NewTimer(timeout)
	defer timeoutTimer.Stop()

	retryTimer := o.clock.NewTimer(0)
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

// Create implements environs.Environ.
func (e *Environ) Create(params environs.CreateParams) error {
	if err := e.cli.Ping(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AdoptResources implements environs.Environ.
func (e *Environ) AdoptResources(controllerUUID string, fromVersion version.Number) error {
	return nil
}

// ConstraintsValidator implements environs.Environ.
func (e *Environ) ConstraintsValidator() (constraints.Validator, error) {
	// list of unsupported OCI provider constraints
	unsupportedConstraints := []string{
		constraints.Container,
		constraints.CpuPower,
		constraints.RootDisk,
		constraints.VirtType,
		constraints.Tags,
	}

	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64})
	logger.Infof("Returning constraints validator: %v", validator)
	return validator, nil
}

// SetConfig implements environs.Environ.
func (e *Environ) SetConfig(cfg *config.Config) error {
	ecfg, err := e.p.newConfig(cfg)
	if err != nil {
		return err
	}

	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.ecfgObj = ecfg

	return nil
}

func (e *Environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	return e.ecfgObj
}

func (e *Environ) allInstances(tags map[string]string) ([]*ociInstance, error) {
	compartment := e.ecfg().compartmentID()
	request := ociCore.ListInstancesRequest{
		CompartmentId: compartment,
	}
	response, err := e.cli.ListInstances(context.Background(), request)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ret := []*ociInstance{}
	for _, val := range response.Items {
		if val.LifecycleState == ociCore.InstanceLifecycleStateTerminated {
			continue
		}
		missingTag := false
		for i, j := range tags {
			tagVal, ok := val.FreeFormTags[i]
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
			return nil, errors.Trace(err)
		}
		ret = append(ret, inst)
	}
	return ret, nil
}

func (e *Environ) allControllerManagedInstances(controllerUUID string) ([]*ociInstance, error) {
	tags := map[string]string{
		tags.JujuController: controllerUUID,
	}
	return e.allInstances(tags)
}

// ControllerInstances implements environs.Environ.
func (e *Environ) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	tags := map[string]string{
		tags.JujuController:   controllerUUID,
		tags.JujuIsController: "true",
	}
	instances, err := e.allInstances(tags)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := []instance.Id{}
	for _, val := range instances {
		ids = append(ids, val.Id())
	}
	return ids, nil
}

// Destroy implements environs.Environ.
func (e *Environ) Destroy() error {
	return common.Destroy(e)
}

// DestroyController implements environs.Environ.
func (e *Environ) DestroyController(controllerUUID string) error {
	err := e.Destroy()
	if err != nil {
		logger.Errorf("Failed to destroy environment through controller: %s", errors.Trace(err))
	}
	instances, err := e.allControllerManagedInstances(controllerUUID)
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

	err = e.StopInstances(ids...)
	if err != nil {
		return err
	}
	logger.Debugf("Cleaning up network resources")
	return e.cleanupNetworksAndSubnets(controllerUUID)
}

// Provider implements environs.Environ.
func (e *Environ) Provider() environs.EnvironProvider {
	return e.p
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (e *Environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{ociStorageProviderType}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (e *Environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == ociStorageProviderType {
		return &storageProvider{
			env: e,
			api: e.cli,
		}, nil
	}

	return nil, errors.NotFoundf("storage provider %q", t)
}

// getCloudInitConfig returns a CloudConfig instance. The default oracle images come
// bundled with iptables-persistent on Ubuntu and firewalld on CentOS, which maintains
// a number of iptables firewall rules. We need to at least allow the juju API port for state
// machines. SSH port is allowed by default on linux images.
func (e *Environ) getCloudInitConfig(series string, apiPort int) (cloudinit.CloudConfig, error) {
	// TODO (gsamfira): remove this function when the above mention bug is fixed
	cloudcfg, err := cloudinit.New(series)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}

	if apiPort == 0 {
		return cloudcfg, nil
	}

	operatingSystem, err := jujuseries.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch operatingSystem {
	case os.Ubuntu:
		fwCmd := fmt.Sprintf(
			"/sbin/iptables -I INPUT -p tcp --dport %d -j ACCEPT", apiPort)
		cloudcfg.AddRunCmd(fwCmd)
		cloudcfg.AddScripts("/etc/init.d/netfilter-persistent save")
	case os.CentOS:
		fwCmd := fmt.Sprintf("firewall-cmd --zone=public --add-port=%d/tcp --permanent", apiPort)
		cloudcfg.AddRunCmd(fwCmd)
	}
	return cloudcfg, nil
}

// StartInstance implements environs.InstanceBroker.
func (e *Environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// var types []instances.InstanceType

	if args.ControllerUUID == "" {
		return nil, errors.NotFoundf("Controller UUID")
	}

	networks, err := e.ensureNetworksAndSubnets(args.ControllerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	zones, err := e.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}

	zone := zones[0].Name()
	network := networks[zone][0]
	// refresh the global image cache
	// this only hits the API every 30 minutes, otherwise just retrieves
	// from cache
	imgCache, err := refreshImageCache(e.cli, e.ecfg().compartmentID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("Image cache contains: %v", imgCache)
	series := args.Tools.OneSeries()
	arches := args.Tools.Arches()

	types := imgCache.supportedShapes(series)

	defaultType := string(VirtualMachine)
	if args.Constraints.VirtType == nil {
		args.Constraints.VirtType = &defaultType
	}

	// check if we find an image that is compliant with the
	// constraints provided in the oracle cloud account
	args.ImageMetadata = imgCache.imageMetadata(series, *args.Constraints.VirtType)

	spec, image, err := findInstanceSpec(
		args.ImageMetadata,
		types,
		&instances.InstanceConstraint{
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
		e.Config(),
	); err != nil {
		return nil, errors.Trace(err)
	}
	hostname, err := e.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := args.InstanceConfig.Tags

	var apiPort int
	var desiredStatus ociCore.InstanceLifecycleStateEnum
	// Wait for controller to actually be running
	if args.InstanceConfig.Controller != nil {
		apiPort = args.InstanceConfig.Controller.Config.APIPort()
		desiredStatus = ociCore.InstanceLifecycleStateRunning
	} else {
		desiredStatus = ociCore.InstanceLifecycleStateProvisioning
	}

	cloudcfg, err := e.getCloudInitConfig(series, apiPort)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}

	// compose userdata with the cloud config template
	logger.Debugf("Composing userdata")
	userData, err := providerinit.ComposeUserData(
		args.InstanceConfig,
		cloudcfg,
		OCIRenderer{},
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}

	// NOTE(gsamfira): Should we make this configurable?
	// TODO(gsamfira): select Availability Domain and subnet ID
	assignPublicIp := true
	instanceDetails := ociCore.LaunchInstanceDetails{
		AvailabilityDomain: &zone,
		CompartmentId:      e.ecfg().compartmentID(),
		ImageId:            &image,
		Shape:              &spec.InstanceType.Name,
		CreateVnicDetails: &ociCore.CreateVnicDetails{
			SubnetId:       network.Id,
			AssignPublicIp: &assignPublicIp,
			DisplayName:    &hostname,
		},
		DisplayName: &hostname,
		Metadata: map[string]string{
			"user_data": string(userData),
		},
		FreeFormTags: tags,
	}

	request := ociCore.LaunchInstanceRequest{
		LaunchInstanceDetails: instanceDetails,
	}

	response, err := e.cli.LaunchInstance(context.Background(), request)
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
	logger.Infof("started instance %q", *machineId)

	if err := instance.waitForPublicIP(); err != nil {
		return nil, errors.Trace(err)
	}

	result := &environs.StartInstanceResult{
		Instance: instance,
		Hardware: instance.hardwareCharacteristics(),
	}

	return result, nil
}

// StopInstances implements environs.InstanceBroker.
func (e *Environ) StopInstances(ids ...instance.Id) error {
	ociInstances, err := e.getOciInstances(ids...)
	if err == environs.ErrNoInstances {
		return nil
	} else if err != nil {
		return err
	}

	logger.Debugf("terminating instances %v", ids)
	if err := e.terminateInstances(ociInstances...); err != nil {
		return err
	}

	return nil
}

func (o *Environ) terminateInstances(instances ...*ociInstance) error {
	wg := sync.WaitGroup{}
	wg.Add(len(instances))
	errs := []error{}
	instIds := []instance.Id{}
	for _, oInst := range instances {
		go func(inst *ociInstance) {
			defer wg.Done()
			if err := inst.deleteInstance(); err != nil {
				instIds = append(instIds, inst.Id())
				errs = append(errs, err)
			} else {
				err := inst.waitForMachineStatus(
					ociCore.InstanceLifecycleStateTerminated,
					5*time.Minute)
				if err != nil {
					instIds = append(instIds, inst.Id())
					errs = append(errs, err)
				}
			}
		}(oInst)
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

// AllInstances implements environs.InstanceBroker.
func (e *Environ) AllInstances() ([]instance.Instance, error) {
	tags := map[string]string{
		tags.JujuModel: e.Config().UUID(),
	}
	instances, err := e.allInstances(tags)
	if err != nil {
		return nil, err
	}

	ret := []instance.Instance{}
	for _, val := range instances {
		ret = append(ret, val)
	}
	return ret, nil
}

// MaintainInstance implements environs.InstanceBroker.
func (e *Environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
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
func (e *Environ) PrecheckInstance(environs.PrecheckInstanceParams) error {
	// var i instances.InstanceTypesWithCostMetadata
	return nil
}

// InstanceTypes implements environs.InstancePrechecker.
func (e *Environ) InstanceTypes(constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, nil
}
