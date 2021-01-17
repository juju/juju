// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packet

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
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
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/retry"
	"github.com/juju/schema"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"github.com/lxc/lxd/shared/logger"
	"github.com/packethost/packngo"
	"gopkg.in/juju/environschema.v1"
)

type environConfig struct {
	config *config.Config
	attrs  map[string]interface{}
}

type environ struct {
	ecfgMutex    sync.Mutex
	ecfg         *environConfig
	name         string
	cloud        environscloudspec.CloudSpec
	packetClient *packngo.Client
}

const (
	packetUserDataOverrides = `#!/bin/bash
rm /etc/ssh/ssh_host_*dsa* 
rm /etc/ssh/ssh_host_ed*
rm /sbin/initctl
sudo apt update
sudo apt install -y dmidecode snapd
set -e
(grep ubuntu /etc/group) || groupadd ubuntu
(id ubuntu &> /dev/null) || useradd -m ubuntu -s /bin/bash -g ubuntu
umask 0077
temp=$(mktemp)
echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > $temp
install -m 0440 $temp /etc/sudoers.d/90-juju-ubuntu
rm $temp
su ubuntu -c 'install -D -m 0600 /dev/null ~/.ssh/authorized_keys'
export authorized_keys="%s"
if [ ! -z "$authorized_keys" ]; then
su ubuntu -c 'printf "$authorized_keys" >> ~/.ssh/authorized_keys'
fi
`
)

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

	devices, _, err := e.packetClient.Devices.List(e.cloud.Credential.Attributes()["project-id"], nil)
	if err != nil {
		return nil, err
	}

	for _, d := range devices {
		cp := d
		if isListContained(packetTags, cp) {
			toReturn = append(toReturn, &packetDevice{e, &cp})
		}
	}

	return toReturn, nil
}

func isListContained(tags []string, d packngo.Device) bool {
	for _, t := range tags {
		tagFound := false
		for _, tt := range d.Tags {
			if t == tt {
				tagFound = true
				break
			}
		}
		if tagFound == false {
			return false
		}
	}
	return true
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
	instanceIDs := []instance.Id{}
	for _, i := range insts {
		instanceIDs = append(instanceIDs, i.Id())
	}
	return nil, nil
}

func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	return nil
}

func (e *environ) Destroy(ctx context.ProviderCallContext) error {
	insts, err := e.getPacketInstancesByTag(map[string]string{"juju-model-uuid": e.Config().UUID()})

	for _, inst := range insts {
		_, err = e.packetClient.Devices.Delete(string(inst.Id()), true)
		if err != nil {
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
		_, err = e.packetClient.Devices.Delete(string(inst.Id()), true)
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (e *environ) InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	var i envinstance.InstanceTypesWithCostMetadata
	return i, nil
}

func (e *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	toReturn := []instances.Instance{}

	tags := []string{"juju-model-uuid=" + e.Config().UUID()}

	for _, id := range ids {
		//TODO handle case when some of the instanes are missing
		d, _, err := e.packetClient.Devices.Get(string(id), nil)
		if err != nil {
			return nil, errors.Annotatef(err, "looking up device with ID %q", id)
		}
		if isListContained(tags, *d) {
			toReturn = append(toReturn, &packetDevice{e, d})
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
	instanceTypes, err := e.supportedInstanceTypes()
	if err != nil {
		return nil, errors.Trace(err)
	}

	spec, err := e.findInstanceSpec(
		args.InstanceConfig.Controller != nil,
		args.ImageMetadata,
		instanceTypes,
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

	juserdata, err := providerinit.ComposeUserData(args.InstanceConfig, nil, PacketRenderer{})

	userdata := strings.Join(
		[]string{
			fmt.Sprintf(packetUserDataOverrides, e.ecfg.config.AuthorizedKeys()),
			strings.ReplaceAll(string(juserdata), "#!/bin/bash", ""),
		}, "\n",
	)

	packetTags := []string{}

	for k, v := range args.InstanceConfig.Tags {
		packetTags = append(packetTags, fmt.Sprintf("%s=%s", k, v))
	}

	device := &packngo.DeviceCreateRequest{
		Hostname:     e.name,
		Facility:     []string{e.cloud.Region},
		Plan:         spec.InstanceType.Name,
		OS:           spec.Image.Id,
		ProjectID:    e.cloud.Credential.Attributes()["project-id"],
		BillingCycle: "hourly",
		UserData:     userdata,
		Tags:         packetTags,
	}

	d, _, err := e.packetClient.Devices.Create(device)
	if err != nil {
		return nil, errors.Trace(err)
	}

	d, err = waitDeviceActive(e.packetClient, d.ID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := &packetDevice{e, d}
	amd64 := arch.AMD64
	mem, err := strconv.ParseUint(d.Plan.Specs.Memory.Total[:len(d.Plan.Specs.Memory.Total)-2], 10, 64)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cpus uint64
	if len(inst.Plan.Specs.Cpus) > 0 {
		cpus = uint64(inst.Plan.Specs.Cpus[0].Count)
	}

	hc := &instance.HardwareCharacteristics{
		Arch: &amd64,
		Mem:  &mem,
		// RootDisk: &instanceSpec.InstanceType.RootDisk,
		CpuCores: &cpus,
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hc,
	}, nil
}

// supportedInstanceTypes returns the instance types supported by packet.
func (e *environ) supportedInstanceTypes() ([]instances.InstanceType, error) {
	plans, _, err := e.packetClient.Plans.List(new(packngo.ListOptions))
	if err != nil {
		return nil, errors.Annotate(err, "retrieving supported instance types")
	}

	var instTypes []instances.InstanceType
nextPlan:
	for _, plan := range plans {
		if !validPlan(plan) {
			continue
		}

		// TODO: Only work with this plan type for now to avoid
		// spinning up more expensive instances while testing.
		if plan.Name != "t1.small.x86" {
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

func validPlan(plan packngo.Plan) bool {
	isInvalid := plan.Pricing == nil ||
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
	// TODO: remove this when we image discovery via streams works
	suitableImages := []*imagemetadata.ImageMetadata{
		{
			Id:   "ubuntu_18_04",
			Arch: arch.AMD64,
		},
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
	return errors.NotImplementedf("Stop instances")
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
