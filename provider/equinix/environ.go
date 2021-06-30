// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
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
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"

	"github.com/packethost/packngo"
	errr "github.com/pkg/errors"
)

//go:generate go run github.com/golang/mock/mockgen -destination ./mocks/packngo.go -package mocks github.com/packethost/packngo DeviceService,OSService,PlanService,ProjectIPService

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
	return e.getPacketInstancesByTag(map[string]string{"juju-model-uuid": e.Config().UUID()})
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
	insts, err := e.getPacketInstancesByTag(map[string]string{
		"juju-is-controller":   "true",
		"juju-controller-uuid": controllerUUID,
	})
	if err != nil {
		return nil, err
	}
	instanceIDs := make([]instance.Id, len(insts))
	for i, inst := range insts {
		instanceIDs[i] = inst.Id()
	}
	return instanceIDs, nil
}

func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	return nil
}

func (e *environ) Destroy(ctx context.ProviderCallContext) error {
	insts, err := e.getPacketInstancesByTag(map[string]string{
		"juju-model-uuid": e.Config().UUID()})
	if err != nil {
		return errors.Trace(err)
	}

	ids := []instance.Id{}

	for _, i := range insts {
		ids = append(ids, i.Id())
	}

	err = e.deleteDevicesByIDs(ctx, ids)
	if err != nil {
		return err
	}

	return common.Destroy(e, ctx)
}

func (e *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	insts, err := e.ControllerInstances(ctx, controllerUUID)
	if err != nil {
		return err
	}

	err = e.deleteDevicesByIDs(ctx, insts)
	if err != nil {
		return err
	}

	return e.Destroy(ctx)
}

func (e *environ) InstanceTypes(context.ProviderCallContext, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	i := instances.InstanceTypesWithCostMetadata{}
	instances, err := e.supportedInstanceTypes()
	if err != nil {
		return i, errors.Trace(err)
	}

	i.InstanceTypes = instances
	return i, nil
}

func (e *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	toReturn := make([]instances.Instance, len(ids))
	var missingInstanceCount int

	tags := set.NewStrings("juju-model-uuid=" + e.Config().UUID())

	for i, id := range ids {
		d, resp, err := e.equinixClient.Devices.Get(string(id), nil)
		if err != nil && resp != nil && resp.Request.Response.StatusCode == http.StatusNotFound {
			logger.Warningf("instance %s not found", string(id))
			missingInstanceCount = missingInstanceCount + 1
			continue
		} else if err != nil {
			return nil, errors.Annotatef(err, "looking up device with ID %q", id)
		}

		deviceTags := set.NewStrings(d.Tags...)
		if tags.Intersection(deviceTags).IsEmpty() {
			missingInstanceCount++
			continue
		}
		toReturn[i] = &equinixDevice{e, d}
	}

	if missingInstanceCount > 0 {
		if missingInstanceCount == len(toReturn) {
			return nil, environs.ErrNoInstances
		}
		return toReturn, environs.ErrPartialInstances

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
			"DEBIAN_FRONTEND=noninteractive apt-get --option=Dpkg::Options::=--force-confdef --option=Dpkg::Options::=--force-confold --option=Dpkg::Options::=--force-unsafe-io --assume-yes --quiet install dmidecode snapd",
			"snap install lxd && sudo adduser ubuntu lxd",
		)
	}

	// NOTE(achilleasa): The following script applies a set of equinix-specific
	// networking fixes:
	//
	// 1) The equinix provisioner creates a /8 route for private IP addresses.
	// As a result, lxd is unable to find a suitable subnet to use when
	// juju runs "lxd init --auto" and effectively prevents workloads from
	// being deployed to equinix machines.
	//
	// The following fixup script queries the equinix metadata service and
	// replaces these problematic routes with the correct ones based on the
	// reserved block(s) that provide addresses to the machine.
	//
	// Another oddity inherent in equinix's network setup is that a sub-block
	// is carved out of the reserved block and gets assigned to the machine.
	// For instance, if the parent block is a /26, the machine gets a /31
	// contained within the parent block. The route fixup script takes this
	// into account and adds the right route based on the metadata details
	// so we can route traffic to any other machine in the parent block.
	//
	// 2) The equinix provider requires the use of FAN for allowing
	// container to container communication across nodes. Due to the way
	// that the networking is configured, we need to install iptable rules
	// to masquerade any non-FAN traffic so that containers get internet
	// connectivity.  As Juju sets up FAN bridges via a worker, we run a
	// script that waits for the bridge to appear and then makes the
	// required iptable changes.
	//
	// The script is run once during provisioning and a cronjob is set up
	// to ensure that it runs after each reboot.
	cloudCfg.AddScripts(`cat << 'EOF' >> /root/juju-fixups.sh
#!/bin/bash

curl -vs http://metadata.packet.net/metadata 2>/dev/null |
jq -r '.network.addresses | .[] | select(.public == false) | [.gateway, .parent_block.network, .parent_block.cidr] | @tsv' |
awk '{print $1" "$2" "$3}' |
while read -r gw net cidr; do
    match=$(ip route show to match ${net} | grep -v default)
    cur_route=$(echo -n ${match} | awk '{print $1}')
    via_dev=$(echo -n ${match} | awk '{print $5}')
    [ -z "$cur_route" ] && continue

    echo "[juju fixup] replacing existing route ${cur_route} (via ${via_dev}) with ${net}/${cidr} (via ${via_dev})"
    ip route del ${cur_route}
    ip route add ${net}/${cidr} dev ${via_dev} via ${gw}
done

while true; do
    fan_net=$(ip route | grep fan | awk '{print $1}')
    if [ -z "$fan_net" ]; then
        sleep 15
        continue
    fi

    masq_rule=$(iptables -t nat -S POSTROUTING | egrep "${fan_net}.*MASQUERADE")
    if [ -z "$masq_rule" ]; then
        echo "[juju fixup] installing iptables rules to masquerade FAN traffic for destinations other than $fan_net"
        iptables -t nat -D POSTROUTING -s $fan_net -j fan-egress
        iptables -t nat -I POSTROUTING -s $fan_net -d $fan_net -j fan-egress
        iptables -t nat -I POSTROUTING -s $fan_net \! -d $fan_net -j MASQUERADE
    fi
    exit 0
done
EOF`,
		"(crontab -l 2>/dev/null; echo '@reboot bash /root/juju-fixups.sh') | crontab -",
		"sh /root/juju-fixups.sh &", // the fixup script includes a polling section so it must be daemonized.
	)

	userdata, err := providerinit.ComposeUserData(args.InstanceConfig, cloudCfg, EquinixRenderer{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Render the required tags for the instance.
	var packetTags []string
	for k, v := range args.InstanceConfig.Tags {
		packetTags = append(packetTags, fmt.Sprintf("%s=%s", k, v))
	}

	hostname, err := e.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	device := &packngo.DeviceCreateRequest{
		Hostname:     hostname,
		Metro:        e.cloud.Region,
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

	allocatedPublicIP := true
	if args.Constraints.HasAllocatePublicIP() {
		allocatedPublicIP = *args.Constraints.AllocatePublicIP
	}

	var requestedPublicAddr, requestedPrivateAddr bool
	if len(subnetIDs) != 0 {
		logger.Debugf("requesting a machine with address in subnet(s): %v", subnetIDs)
		for _, subnetID := range subnetIDs {
			net, _, err := e.equinixClient.ProjectIPs.Get(subnetID, &packngo.GetOptions{})
			if err != nil {
				return nil, errors.Trace(err)
			}

			requestedPublicAddr = requestedPublicAddr || net.Public
			requestedPrivateAddr = requestedPrivateAddr || !net.Public

			if !allocatedPublicIP && net.Public {
				continue
			}

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
	if allocatedPublicIP && !requestedPublicAddr {
		// Allocate a public address from the default address pool.
		device.IPAddresses = append(device.IPAddresses, packngo.IPAddressCreateRequest{
			Public:        true,
			AddressFamily: 4,
			CIDR:          31,
		})
	}

	d, _, err := e.equinixClient.Devices.Create(device)
	if err != nil {
		return nil, errors.Trace(err)
	}

	d, err = waitDeviceActive(ctx, e.equinixClient, d.ID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := &equinixDevice{e, d}
	mem, err := strconv.ParseUint(d.Plan.Specs.Memory.Total[:len(d.Plan.Specs.Memory.Total)-2], 10, 64)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cpus uint64 = 1
	if inst.Plan != nil && inst.Plan.Specs != nil && len(inst.Plan.Specs.Cpus) > 0 {
		cpus = uint64(inst.Plan.Specs.Cpus[0].Count)
	}

	arch := getArchitectureFromPlan(d.Plan.Name)

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: &instance.HardwareCharacteristics{
			Arch: &arch,
			Mem:  &mem,
			// RootDisk: &instanceSpec.InstanceType.RootDisk,
			CpuCores: &cpus,
		},
	}, nil
}

func (e *environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	return e.deleteDevicesByIDs(ctx, ids)
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

func (e *environ) deleteDevicesByIDs(ctx context.ProviderCallContext, ids []instance.Id) error {
	for _, id := range ids {
		_, err := e.equinixClient.Devices.Delete(string(id), true)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// if values tag and state are left empty it will return all instances
func (e *environ) getPacketInstancesByTag(tags map[string]string) ([]instances.Instance, error) {
	var toReturn []instances.Instance

	deviceTags := set.NewStrings()
	for k, v := range tags {
		deviceTags.Add(fmt.Sprintf("%s=%s", k, v))
	}

	projectID, ok := e.cloud.Credential.Attributes()["project-id"]
	if !ok {
		return nil, fmt.Errorf("project-id not fond as attribute")
	}

	devices, _, err := e.equinixClient.Devices.List(projectID, nil)
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
			logger.Tracef("Plan %s not valid in facility %s", plan.Name, e.cloud.Region)
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
				// The Equinix Metal API returns all plan as legacy today. There is an issue open internally to figure out why.
				// In the meantime let's comment this https://github.com/juju/juju/pull/12983#discussion_r635324484
				// Deprecated: plan.Legacy,
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
	spec, err := instances.FindInstanceSpec(images, ic, instanceTypes)
	if err != nil {
		return nil, err
	}
	return spec, err
}

func (e *environ) finishInstanceConfig(args *environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	matchingTools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return errors.Errorf("chosen architecture %v for image %q not present in %v",
			spec.Image.Arch, spec.Image.Id, args.Tools.Arches())
	}

	if spec.InstanceType.Deprecated {
		logger.Warningf("deprecated instance type specified: %s", spec.InstanceType.Name)
	}

	if err := args.InstanceConfig.SetTools(matchingTools); err != nil {
		return errors.Trace(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, e.Config()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

var ErrDeviceProvisioningFailed = errors.New("device provisioning failed")

// waitDeviceActive is a function capable of figuring out when a Equinix Metal
// device is active
func waitDeviceActive(ctx context.ProviderCallContext, c *packngo.Client, id string) (*packngo.Device, error) {
	var d *packngo.Device
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			var err error
			d, _, err = c.Devices.Get(id, nil)
			if err != nil {
				return err
			}
			if d.State == "active" {
				return nil
			}
			if d.State == "failed" {
				return errr.Wrap(ErrDeviceProvisioningFailed, fmt.Sprintf("device %s provisioning failed", id))
			}
			return fmt.Errorf("device in not in active state yet")
		},
		IsFatalError: func(err error) bool {
			if errr.Is(err, ErrDeviceProvisioningFailed) {
				return true
			}
			return common.IsCredentialNotValid(err)
		},
		Attempts: 180,
		Delay:    5 * time.Second,
		Clock:    clock.WallClock,
	})

	if d != nil {
		return d, nil
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
