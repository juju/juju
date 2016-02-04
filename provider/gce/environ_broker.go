// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/series"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

func isController(icfg *instancecfg.InstanceConfig) bool {
	return multiwatcher.AnyJobNeedsState(icfg.Jobs...)
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// Please note that in order to fulfil the demands made of Instances and
	// AllInstances, it is imperative that some environment feature be used to
	// keep track of which instances were actually started by juju.
	env = env.getSnapshot()

	// Start a new instance.

	if args.InstanceConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}

	spec, err := buildInstanceSpec(env, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := env.finishInstanceConfig(args, spec); err != nil {
		return nil, errors.Trace(err)
	}

	raw, err := newRawInstance(env, args, spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("started instance %q in zone %q", raw.ID, raw.ZoneName)
	inst := newInstance(raw, env)

	// Ensure the API server port is open (globally for all instances
	// on the network, not just for the specific node of the state
	// server). See LP bug #1436191 for details.
	if isController(args.InstanceConfig) {
		ports := network.PortRange{
			FromPort: args.InstanceConfig.StateServingInfo.APIPort,
			ToPort:   args.InstanceConfig.StateServingInfo.APIPort,
			Protocol: "tcp",
		}
		if err := env.gce.OpenPorts(env.globalFirewallName(), ports); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Build the result.
	hwc := getHardwareCharacteristics(env, spec, inst)
	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

var buildInstanceSpec = func(env *environ, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	return env.buildInstanceSpec(args)
}

var newRawInstance = func(env *environ, args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	return env.newRawInstance(args, spec)
}

var getHardwareCharacteristics = func(env *environ, spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	return env.getHardwareCharacteristics(spec, inst)
}

// finishInstanceConfig updates args.InstanceConfig in place. Setting up
// the API, StateServing, and SSHkeys information.
func (env *environ) finishInstanceConfig(args environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	envTools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return errors.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	args.InstanceConfig.Tools = envTools[0]
	return instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config())
}

// buildInstanceSpec builds an instance spec from the provided args
// and returns it. This includes pulling the simplestreams data for the
// machine type, region, and other constraints.
func (env *environ) buildInstanceSpec(args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	arches := args.Tools.Arches()
	series := args.Tools.OneSeries()
	spec, err := findInstanceSpec(
		env, &instances.InstanceConstraint{
			Region:      env.ecfg.region(),
			Series:      series,
			Arches:      arches,
			Constraints: args.Constraints,
		},
		args.ImageMetadata,
	)
	return spec, errors.Trace(err)
}

var findInstanceSpec = func(
	env *environ,
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
) (*instances.InstanceSpec, error) {
	return env.findInstanceSpec(ic, imageMetadata)
}

// findInstanceSpec initializes a new instance spec for the given
// constraints and returns it. This only covers populating the
// initial data for the spec.
func (env *environ) findInstanceSpec(
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
) (*instances.InstanceSpec, error) {
	images := instances.ImageMetadataToImages(imageMetadata)
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	return spec, errors.Trace(err)
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	machineID := common.MachineFullName(env, args.InstanceConfig.MachineId)

	os, err := series.GetOSFromSeries(args.InstanceConfig.Series)
	if err != nil {
		return nil, errors.Trace(err)
	}

	metadata, err := getMetadata(args, os)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := []string{
		env.globalFirewallName(),
		machineID,
	}

	cfg := env.Config()
	eUUID, ok := cfg.UUID()
	if !ok {
		return nil, errors.NotFoundf("UUID necessary to create the instance disk")
	}

	disks, err := getDisks(spec, args.Constraints, args.InstanceConfig.Series, eUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Use the env ID for the network name (instead of default)?
	// TODO(ericsnow) Make the network name configurable?
	// TODO(ericsnow) Support multiple networks?
	// TODO(ericsnow) Use a different net interface name? Configurable?
	instSpec := google.InstanceSpec{
		ID:                machineID,
		Type:              spec.InstanceType.Name,
		Disks:             disks,
		NetworkInterfaces: []string{"ExternalNAT"},
		Metadata:          metadata,
		Tags:              tags,
		// Network is omitted (left empty).
	}

	zones, err := env.parseAvailabilityZones(args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst, err := env.gce.AddInstance(instSpec, zones...)
	return inst, errors.Trace(err)
}

// getMetadata builds the raw "user-defined" metadata for the new
// instance (relative to the provided args) and returns it.
func getMetadata(args environs.StartInstanceParams, os jujuos.OSType) (map[string]string, error) {
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, nil, GCERenderer{})
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("GCE user data; %d bytes", len(userData))

	metadata := make(map[string]string)
	if isController(args.InstanceConfig) {
		metadata[metadataKeyIsState] = metadataValueTrue
	} else {
		metadata[metadataKeyIsState] = metadataValueFalse
	}
	switch os {
	case jujuos.Ubuntu:
		// We store a gz snapshop of information that is used by
		// cloud-init and unpacked in to the /var/lib/cloud/instances folder
		// for the instance. Due to a limitation with GCE and binary blobs
		// we base64 encode the data before storing it.
		metadata[metadataKeyCloudInit] = string(userData)
		// Valid encoding values are determined by the cloudinit GCE data source.
		// See: http://cloudinit.readthedocs.org
		metadata[metadataKeyEncoding] = "base64"

		authKeys, err := google.FormatAuthorizedKeys(args.InstanceConfig.AuthorizedKeys, "ubuntu")
		if err != nil {
			return nil, errors.Trace(err)
		}

		metadata[metadataKeySSHKeys] = authKeys
	case jujuos.Windows:
		metadata[metadataKeyWindowsUserdata] = string(userData)

		validChars := append(utils.UpperAlpha, append(utils.LowerAlpha, utils.Digits...)...)

		// The hostname must have maximum 15 characters
		winHostname := "juju" + utils.RandomString(11, validChars)
		metadata[metadataKeyWindowsSysprep] = fmt.Sprintf(winSetHostnameScript, winHostname)
	default:
		return nil, errors.Errorf("cannot pack metadata for os %s on the gce provider", os.String())
	}

	return metadata, nil
}

// getDisks builds the raw spec for the disks that should be attached to
// the new instances and returns it. This will always include a root
// disk with characteristics determined by the provides args and
// constraints.
func getDisks(spec *instances.InstanceSpec, cons constraints.Value, ser, eUUID string) ([]google.DiskSpec, error) {
	size := common.MinRootDiskSizeGiB(ser)
	if cons.RootDisk != nil && *cons.RootDisk > size {
		size = common.MiBToGiB(*cons.RootDisk)
	}
	var imageURL string
	os, err := series.GetOSFromSeries(ser)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch os {
	case jujuos.Ubuntu:
		imageURL = ubuntuImageBasePath
	case jujuos.Windows:
		imageURL = windowsImageBasePath
	default:
		return nil, errors.Errorf("os %s is not supported on the gce provider", os.String())
	}
	dSpec := google.DiskSpec{
		Series:      ser,
		SizeHintGB:  size,
		ImageURL:    imageURL + spec.Image.Id,
		Boot:        true,
		AutoDelete:  true,
		Description: eUUID,
	}
	if cons.RootDisk != nil && dSpec.TooSmall() {
		msg := "Ignoring root-disk constraint of %dM because it is smaller than the GCE image size of %dG"
		logger.Infof(msg, *cons.RootDisk, google.MinDiskSizeGB(ser))
	}
	return []google.DiskSpec{dSpec}, nil
}

// getHardwareCharacteristics compiles hardware-related details about
// the given instance and relative to the provided spec and returns it.
func (env *environ) getHardwareCharacteristics(spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	rootDiskMB := inst.base.RootDiskGB() * 1024
	hwc := instance.HardwareCharacteristics{
		Arch:             &spec.Image.Arch,
		Mem:              &spec.InstanceType.Mem,
		CpuCores:         &spec.InstanceType.CpuCores,
		CpuPower:         spec.InstanceType.CpuPower,
		RootDisk:         &rootDiskMB,
		AvailabilityZone: &inst.base.ZoneName,
		// Tags: not supported in GCE.
	}
	return &hwc
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances() ([]instance.Instance, error) {
	instances, err := getInstances(env)
	return instances, errors.Trace(err)
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(instances ...instance.Id) error {
	env = env.getSnapshot()

	var ids []string
	for _, id := range instances {
		ids = append(ids, string(id))
	}

	prefix := common.MachineFullName(env, "")
	err := env.gce.RemoveInstances(prefix, ids...)
	return errors.Trace(err)
}
