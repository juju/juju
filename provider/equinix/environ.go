// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	envinstance "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/retry"
	"github.com/juju/schema"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/environschema.v1"

	"github.com/packethost/packngo"
)

var logger = loggo.GetLogger("juju.provider.equnix")

type environConfig struct {
	config *config.Config
	attrs  map[string]interface{}
}

type environ struct {
	ecfgMutex    sync.Mutex
	ecfg         *environConfig
	name         string
	cloud        environscloudspec.CloudSpec
	equnixClient *packngo.Client
	namespace    instance.Namespace
}

// func newEMEnviron() (*environ, error) {
// 	e := new(environ)
// 	namespace, err := instance.NewNamespace(e.ecfg.config.UUID())
// 	if err != nil {
// 		return nil, errors.Trace(err)
// 	}

// 	e.ecfg, err = providerInstance.newConfig(cfg)
// 	if err != nil {
// 		return nil, errors.Trace(err)
// 	}

// 	e.namespace = namespace
// 	return e, nil
// }

var providerInstance environProvider

func (e *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	return nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, callCtx, args)
}

func (e *environ) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.getPacketInstancesByTag(map[string]string{"juju-model-uuid": e.Config().UUID()})
}

// if values tag and state are left empty it will return all instances
func (e *environ) getPacketInstancesByTag(tags map[string]string) ([]instances.Instance, error) {
	toReturn := []instances.Instance{}
	packetTags := []string{}

	for k, v := range tags {
		packetTags = append(packetTags, fmt.Sprintf("%s=%s", k, v))
	}
	deviceTags := set.NewStrings(packetTags...)
	devices, _, err := e.equnixClient.Devices.List(e.cloud.Credential.Attributes()["project-id"], nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, d := range devices {
		cp := d
		cpTags := set.NewStrings(cp.Tags...)
		if !deviceTags.Intersection(cpTags).IsEmpty() {
			toReturn = append(toReturn, &equnixDevice{e, &cp})
		}
	}

	return toReturn, nil
}

func (e *environ) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.getPacketInstancesByTag(map[string]string{"juju-model-uuid": e.Config().UUID()})
}

func (e *environ) Config() *config.Config {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	return e.ecfg.config
}

func (e *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported([]string{constraints.CpuPower, constraints.VirtType})
	validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.I386, arch.PPC64EL})
	return validator, nil
}

func (e *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	insts, err := e.getPacketInstancesByTag(map[string]string{"juju-is-controller": "true", "juju-controller-uuid": controllerUUID})
	if err != nil {
		return nil, err
	}
	instanceIDs := make([]instance.Id, len(insts))
	for _, i := range insts {
		instanceIDs = append(instanceIDs, i.Id())
	}
	return instanceIDs, nil
}

func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	return nil
}

func (e *environ) Destroy(ctx context.ProviderCallContext) error {
	insts, err := e.getPacketInstancesByTag(map[string]string{"juju-model-uuid": e.Config().UUID()})
	if err != nil {
		return errors.Trace(err)
	}

	for _, inst := range insts {
		if _, err = e.equnixClient.Devices.Delete(string(inst.Id()), true); err != nil {
			return errors.Trace(err)
		}
	}

	return common.Destroy(e, ctx)
}

func (e *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	insts, err := e.getPacketInstancesByTag(map[string]string{"juju-controller-uuid": controllerUUID})
	if err != nil {
		return err
	}

	for _, inst := range insts {
		_, err = e.equnixClient.Devices.Delete(string(inst.Id()), true)
		if err != nil {
			return errors.Trace(err)
		}
	}

	return e.Destroy(ctx)
}

func (e *environ) InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	i := envinstance.InstanceTypesWithCostMetadata{}
	instances, err := e.supportedInstanceTypes()
	if err != nil {
		return i, errors.Trace(err)
	}

	i.InstanceTypes = instances
	return i, nil
}

func (e *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	toReturn := []instances.Instance{}

	tags := set.NewStrings("juju-model-uuid=" + e.Config().UUID())

	for _, id := range ids {
		//TODO handle case when some of the instanes are missing
		d, _, err := e.equnixClient.Devices.Get(string(id), nil)
		if err != nil {
			return nil, errors.Annotatef(err, "looking up device with ID %q", id)
		}
		deviceTags := set.NewStrings(d.Tags...)
		if !tags.Intersection(deviceTags).IsEmpty() {
			toReturn = append(toReturn, &equnixDevice{e, d})
		}
	}
	if len(toReturn) == 0 {
		return nil, environs.ErrNoInstances
	}
	return toReturn, nil
}

func (e *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	return nil
}

func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	e.name = controllerName
	return nil
}

func (*environ) Provider() environs.EnvironProvider {
	return &environProvider{}
}

func (e *environ) SetConfig(cfg *config.Config) error {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return errors.Annotate(err, "invalid config change")
	}
	e.ecfg = ecfg
	return nil
}

var configImmutableFields = []string{}
var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()
var configSchema = environschema.Fields{}
var configDefaults = schema.Defaults{}

func newConfig(cfg, old *config.Config) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, old); err != nil {
		return nil, errors.Trace(err)
	}
	attrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if old != nil {
		// There's an old configuration. Validate it so that any
		// default values are correctly coerced for when we check
		// the old values later.
		oldEcfg, err := newConfig(old, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid base config")
		}
		for _, attr := range configImmutableFields {
			oldv, newv := oldEcfg.attrs[attr], attrs[attr]
			if oldv != newv {
				return nil, errors.Errorf(
					"%s: cannot change from %v to %v",
					attr, oldv, newv,
				)
			}
		}
	}

	ecfg := &environConfig{
		config: cfg,
		attrs:  attrs,
	}
	return ecfg, nil
}

func (e *environ) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (result *environs.StartInstanceResult, resultErr error) {
	instanceTypes, err := e.InstanceTypes(ctx, constraints.Value{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	spec, err := e.findInstanceSpec(
		args.InstanceConfig.Controller != nil,
		args.ImageMetadata,
		instanceTypes.InstanceTypes,
		&instances.InstanceConstraint{
			Region:      e.cloud.Region,
			Series:      args.InstanceConfig.Series,
			Arches:      args.Tools.Arches(),
			Constraints: args.Constraints,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := e.finishInstanceConfig(&args, spec); err != nil {
		return nil, errors.Trace(err)
	}

	cloudCfg, err := cloudinit.New(args.InstanceConfig.Series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudCfg.AddScripts(
		// This is a dummy script injected into packet images that
		// confuses the init system detection logic used by juju.
		"rm -f /sbin/initctl",
	)

	// Install additional dependencies that are present in ubuntu images
	// but not in the versions built by equinix.
	//
	// NOTE(achilleasa): this is a hack and is only meant to be used
	// temporarily; we must ensure that equinix mirrors the official
	// ubuntu cloud images.
	if _, err := series.UbuntuSeriesVersion(args.InstanceConfig.Series); err == nil {
		cloudCfg.AddScripts(
			"apt-get update",
			"DEBIAN_FRONTEND=noninteractive apt-get --option=Dpkg::Options::=--force-confdef --option=Dpkg::Options::=--force-confold --option=Dpkg::Options::=--force-unsafe-io --assume-yes --quiet install dmidecode snapd lxd",
		)
	}

	userdata, err := providerinit.ComposeUserData(args.InstanceConfig, cloudCfg, EquinixRenderer{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Render the required tags for the instance.
	packetTags := make([]string, len(args.InstanceConfig.Tags))
	for k, v := range args.InstanceConfig.Tags {
		packetTags = append(packetTags, fmt.Sprintf("%s=%s", k, v))
	}

	hostname, err := e.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	device := &packngo.DeviceCreateRequest{
		Hostname:     hostname,
		Facility:     []string{e.cloud.Region},
		Plan:         spec.InstanceType.Name,
		OS:           spec.Image.Id,
		ProjectID:    e.cloud.Credential.Attributes()["project-id"],
		BillingCycle: "hourly",
		UserData:     string(userdata),
		Tags:         packetTags,
	}

	subnetIDs, err := e.getSubnetsToZoneMap(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var requestedPublicAddr, requestedPrivateAddr bool
	if len(subnetIDs) != 0 {
		logger.Debugf("requesting a machine with address in subnet(s): %v", subnetIDs)
		for _, subnetID := range subnetIDs {
			net, _, err := e.equnixClient.ProjectIPs.Get(subnetID, &packngo.GetOptions{})
			if err != nil {
				return nil, errors.Trace(err)
			}

			requestedPublicAddr = requestedPublicAddr || net.Public
			requestedPrivateAddr = requestedPrivateAddr || !net.Public

			// Packet requires us to request at least a /31 for IPV4
			// addresses and a /127 for IPV6 ones.
			cidrSize := 31
			if net.AddressFamily != 4 {
				cidrSize = 127
			}

			ipBlock := packngo.IPAddressCreateRequest{
				AddressFamily: net.AddressFamily,
				Public:        net.Public,
				CIDR:          cidrSize,
				Reservations:  []string{net.ID},
			}
			device.IPAddresses = append(device.IPAddresses, ipBlock)
		}
	}

	// In order to spin up a new device, we must specify at least one
	// public and one private address.
	if !requestedPrivateAddr {
		// Allocate a private address from the default address pool.
		device.IPAddresses = append(device.IPAddresses, packngo.IPAddressCreateRequest{
			Public:        false,
			AddressFamily: 4,
			CIDR:          31,
		})
	}
	if !requestedPublicAddr {
		// Allocate a public address from the default address pool.
		device.IPAddresses = append(device.IPAddresses, packngo.IPAddressCreateRequest{
			Public:        true,
			AddressFamily: 4,
			CIDR:          31,
		})
	}

	d, _, err := e.equnixClient.Devices.Create(device)
	if err != nil {
		return nil, errors.Trace(err)
	}

	d, err = waitDeviceActive(e.equnixClient, d.ID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := &equnixDevice{e, d}
	amd64 := arch.AMD64
	mem, err := strconv.ParseUint(d.Plan.Specs.Memory.Total[:len(d.Plan.Specs.Memory.Total)-2], 10, 64)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cpus uint64 = 1
	if inst.Plan != nil && inst.Plan.Specs != nil && len(inst.Plan.Specs.Cpus) > 0 {
		cpus = uint64(inst.Plan.Specs.Cpus[0].Count)
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: &instance.HardwareCharacteristics{
			Arch: &amd64,
			Mem:  &mem,
			// RootDisk: &instanceSpec.InstanceType.RootDisk,
			CpuCores: &cpus,
		},
	}, nil
}

func (e *environ) getSubnetsToZoneMap(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	var subnetIDs []string
	for _, subnetList := range args.SubnetsToZones {
		for subnetID := range subnetList {
			packetSubnetID := strings.TrimPrefix(subnetID.String(), "subnet-")
			subnetIDs = append(subnetIDs, packetSubnetID)
		}
	}

	return subnetIDs, nil
}

// supportedInstanceTypes returns the instance types supported by Equnix Metal.
func (e *environ) supportedInstanceTypes() ([]instances.InstanceType, error) {
	opt := &packngo.ListOptions{
		Includes: []string{"available_in"},
	}
	plans, _, err := e.equnixClient.Plans.List(opt)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving supported instance types")
	}

	var instTypes []instances.InstanceType
nextPlan:
	for _, plan := range plans {
		if !validPlan(plan, e.cloud.Region) {
			logger.Infof("Plan %s not valid in facility %s", plan.Name, e.cloud.Region)
			continue
		}

		var instArch string
		switch {
		case strings.HasSuffix(plan.Name, ".x86"):
			instArch = arch.AMD64
		case strings.HasSuffix(plan.Name, ".arm"):
			instArch = arch.ARM64
		default:
			continue nextPlan

		}

		mem, err := parseMemValue(plan.Specs.Memory.Total)
		if err != nil {
			continue
		}

		instTypes = append(instTypes,
			instances.InstanceType{
				Id:       plan.ID,
				Name:     plan.Name,
				CpuCores: uint64(plan.Specs.Cpus[0].Count),
				Mem:      mem,
				Arches:   []string{instArch},
				// Scale per hour costs so they can be represented as an integer for sorting purposes.
				Cost: uint64(plan.Pricing.Hour * 1000.0),
				// TODO: returned by packet's API but not exposed by the packngo client
				// Deprecated: plan.Legacy,
			})
	}

	return instTypes, nil
}

func validPlan(plan packngo.Plan, region string) bool {
	notAvailable := true
	for _, a := range plan.AvailableIn {
		if a.Code == region {
			notAvailable = false
			break
		}
	}
	isInvalid := notAvailable || plan.Pricing == nil ||
		plan.Specs == nil ||
		plan.Specs.Memory == nil ||
		len(plan.Specs.Cpus) == 0 || plan.Specs.Cpus[0].Count == 0

	return !isInvalid
}

func parseMemValue(v string) (uint64, error) {
	var scaler = uint64(1)
	if strings.HasSuffix(v, "GB") {
		scaler = 1024
		v = strings.TrimSuffix(v, "GB")
	}

	val, err := strconv.ParseUint(v, 10, 64)
	return val * scaler, err
}

func (e *environ) findInstanceSpec(controller bool, allImages []*imagemetadata.ImageMetadata, instanceTypes []instances.InstanceType, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	oss, _, err := e.equnixClient.OperatingSystems.List()
	if err != nil {
		return nil, err
	}
	suitableImages := []*imagemetadata.ImageMetadata{}

	for _, it := range instanceTypes {
	nextImage:
		for _, os := range oss {

			switch os.Distro {
			case "ubuntu":
				series, err := series.VersionSeries(os.Version)
				if err != nil || ic.Series != series {
					continue nextImage
				}
			case "centos":
				series, err := series.CentOSVersionSeries(os.Version)
				if err != nil || ic.Series != series {
					continue nextImage
				}
			case "windows":
				series, err := series.WindowsVersionSeries(os.Version)
				if err != nil || ic.Series != series {
					continue nextImage
				}
			default:
				continue nextImage
			}

			for _, p := range os.ProvisionableOn {
				if p == it.Name {
					image := &imagemetadata.ImageMetadata{
						Id:   os.Slug,
						Arch: arch.AMD64,
					}
					suitableImages = append(suitableImages, image)
				}
			}
		}
	}

	images := instances.ImageMetadataToImages(suitableImages)
	return instances.FindInstanceSpec(images, ic, instanceTypes)
}

func (e *environ) finishInstanceConfig(args *environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	matchingTools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return errors.Errorf("chosen architecture %v for image %q not present in %v",
			spec.Image.Arch, spec.Image.Id, args.Tools.Arches())
	}

	if spec.InstanceType.Deprecated {
		logger.Infof("deprecated instance type specified: %s", spec.InstanceType.Name)
	}

	if err := args.InstanceConfig.SetTools(matchingTools); err != nil {
		return errors.Trace(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, e.Config()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (e *environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	for _, id := range ids {
		_, err := e.equnixClient.Devices.Delete(string(id), true)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (e *environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}

func (e *environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}

func waitDeviceActive(c *packngo.Client, id string) (*packngo.Device, error) {
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			d, _, err := c.Devices.Get(id, nil)
			if err != nil {
				return err
			}
			if d.State == "active" {
				return nil
			}
			if d.State == "failed" {
				return fmt.Errorf("device %s provisioning failed", id)
			}
			return nil
		},
		IsFatalError: func(err error) bool {
			return common.IsCredentialNotValid(err)
		},
		Attempts: 180,
		Delay:    5 * time.Second,
		Clock:    clock.WallClock,
	})

	if err == nil {
		d, _, er := c.Devices.Get(id, nil)
		return d, er

	}

	return nil, err
}

// Region is specified in the HasRegion interface.
func (e *environ) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   e.cloud.Region,
		Endpoint: e.cloud.Endpoint,
	}, nil
}
