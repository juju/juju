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
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/schema"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/set"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"

	"github.com/packethost/packngo"
)

var logger = loggo.GetLogger("juju.provider.equinix")

type environConfig struct {
	config *config.Config
	attrs  map[string]interface{}
}

type environ struct {
	ecfgMutex     sync.Mutex
	ecfg          *environConfig
	name          string
	cloud         environscloudspec.CloudSpec
	equinixClient *packngo.Client
	namespace     instance.Namespace
}

var providerInstance environProvider

func (e *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	return nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, callCtx, args)
}

func (e *environ) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) Config() *config.Config {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	return e.ecfg.config
}

func (e *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	return nil, errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	return nil
}

func (e *environ) Destroy(ctx context.ProviderCallContext) error {
	return errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	return errors.NewNotImplemented(nil, "not implemented")
}

func (e *environ) InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

func (e *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	panic(errors.NewNotImplemented(nil, "not implemented"))
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
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

func (e *environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

func (e *environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}

func (e *environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}

// Region is specified in the HasRegion interface.
func (e *environ) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   e.cloud.Region,
		Endpoint: e.cloud.Endpoint,
	}, nil
}

// if values tag and state are left empty it will return all instances
func (e *environ) getPacketInstancesByTag(tags map[string]string) ([]instances.Instance, error) {
	var toReturn []instances.Instance

	deviceTags := set.NewStrings()
	for k, v := range tags {
		deviceTags.Add(fmt.Sprintf("%s=%s", k, v))
	}

	devices, _, err := e.equinixClient.Devices.List(e.cloud.Credential.Attributes()["project-id"], nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, d := range devices {
		cp := d
		cpTags := set.NewStrings(cp.Tags...)
		if !deviceTags.Intersection(cpTags).IsEmpty() {
			toReturn = append(toReturn, &equinixDevice{e, &cp})
		}
	}

	return toReturn, nil
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
		Includes: []string{"available_in_metros"},
	}
	plans, _, err := e.equinixClient.Plans.List(opt)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving supported instance types")
	}

	var instTypes []instances.InstanceType
nextPlan:
	for _, plan := range plans {
		if !validPlan(plan, e.cloud.Region) {
			logger.Debugf("Plan %s not valid in facility %s", plan.Name, e.cloud.Region)
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
				Cost:       uint64(plan.Pricing.Hour * 1000.0),
				Deprecated: plan.Legacy,
			})
	}

	return instTypes, nil
}

func validPlan(plan packngo.Plan, region string) bool {
	// some plans may not be servers
	if plan.Pricing == nil ||
		plan.Specs == nil ||
		plan.Specs.Memory == nil ||
		len(plan.Specs.Cpus) == 0 || plan.Specs.Cpus[0].Count == 0 {
		return false
	}
	for _, a := range plan.AvailableInMetros {
		// some plans are not available in-region
		if a.Code != region {
			continue
		}
		return true
	}
	return false
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
	oss, _, err := e.equinixClient.OperatingSystems.List()
	if err != nil {
		return nil, err
	}
	var suitableImages []*imagemetadata.ImageMetadata

	for _, it := range instanceTypes {
		for _, os := range oss {
			if !isDistroSupported(os, ic) {
				continue
			}

			for _, p := range os.ProvisionableOn {
				if p == it.Name {
					image := &imagemetadata.ImageMetadata{
						Id:   os.Slug,
						Arch: getArchitectureFromPlan(p),
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

// waitDeviceActive is a function capable of figuring out when a Equinix Metal
// device is active
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

// Helper function to get supported OS version
func isDistroSupported(os packngo.OS, ic *instances.InstanceConstraint) bool {
	switch os.Distro {
	case "ubuntu":
		series, err := series.VersionSeries(os.Version)
		if err != nil || ic.Series != series {
			return false
		}
	case "centos":
		series, err := series.CentOSVersionSeries(os.Version)
		if err != nil || ic.Series != series {
			return false
		}
	case "windows":
		series, err := series.WindowsVersionSeries(os.Version)
		if err != nil || ic.Series != series {
			return false
		}
	default:
		return false
	}

	return true
}

// helper function which tries to extract processor architecture from plan name.
// plan names have format like c2.small.arm where in majority of cases the last bit indicates processor architecture.
// in some cases baremeta_1 and similar are returned which are mapped to AMD64.
func getArchitectureFromPlan(p string) string {
	planSplit := strings.Split(p, ".")
	var architecture string
	if len(planSplit) > 2 {
		architecture = planSplit[2]
	}
	switch architecture {
	case "x86":
		return arch.AMD64
	case "arm":
		return arch.ARM64
	default:
		return arch.AMD64
	}
}
