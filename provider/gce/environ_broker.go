// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"encoding/base64"

	"github.com/juju/errors"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

func isStateServer(icfg *instancecfg.InstanceConfig) bool {
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
	if isStateServer(args.InstanceConfig) {
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
	spec, err := findInstanceSpec(env, env.Config().ImageStream(), &instances.InstanceConstraint{
		Region:      env.ecfg.region(),
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
	})
	return spec, errors.Trace(err)
}

var findInstanceSpec = func(env *environ, stream string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	return env.findInstanceSpec(stream, ic)
}

// findInstanceSpec initializes a new instance spec for the given stream
// (and constraints) and returns it. This only covers populating the
// initial data for the spec. However, it does include fetching the
// correct simplestreams image data.
func (env *environ) findInstanceSpec(stream string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: env.cloudSpec(ic.Region),
		Series:    []string{ic.Series},
		Arches:    ic.Arches,
		Stream:    stream,
	})

	signedImageDataOnly := false
	matchingImages, _, err := imageMetadataFetch(sources, imageConstraint, signedImageDataOnly)
	if err != nil {
		return nil, errors.Trace(err)
	}

	images := instances.ImageMetadataToImages(matchingImages)
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	return spec, errors.Trace(err)
}

var imageMetadataFetch = imagemetadata.Fetch

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	machineID := common.MachineFullName(env, args.InstanceConfig.MachineId)

	metadata, err := getMetadata(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := []string{
		env.globalFirewallName(),
		machineID,
	}
	// TODO(ericsnow) Use the env ID for the network name (instead of default)?
	// TODO(ericsnow) Make the network name configurable?
	// TODO(ericsnow) Support multiple networks?
	// TODO(ericsnow) Use a different net interface name? Configurable?
	instSpec := google.InstanceSpec{
		ID:                machineID,
		Type:              spec.InstanceType.Name,
		Disks:             getDisks(spec, args.Constraints),
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
func getMetadata(args environs.StartInstanceParams) (map[string]string, error) {
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("GCE user data; %d bytes", len(userData))

	authKeys, err := google.FormatAuthorizedKeys(args.InstanceConfig.AuthorizedKeys, "ubuntu")
	if err != nil {
		return nil, errors.Trace(err)
	}

	b64UserData := base64.StdEncoding.EncodeToString([]byte(userData))
	metadata := map[string]string{
		metadataKeyIsState: metadataValueFalse,
		// We store a gz snapshop of information that is used by
		// cloud-init and unpacked in to the /var/lib/cloud/instances folder
		// for the instance. Due to a limitation with GCE and binary blobs
		// we base64 encode the data before storing it.
		metadataKeyCloudInit: b64UserData,
		// Valid encoding values are determined by the cloudinit GCE data source.
		// See: http://cloudinit.readthedocs.org
		metadataKeyEncoding: "base64",
		metadataKeySSHKeys:  authKeys,
	}
	if isStateServer(args.InstanceConfig) {
		metadata[metadataKeyIsState] = metadataValueTrue
	}

	return metadata, nil
}

// getDisks builds the raw spec for the disks that should be attached to
// the new instances and returns it. This will always include a root
// disk with characteristics determined by the provides args and
// constraints.
func getDisks(spec *instances.InstanceSpec, cons constraints.Value) []google.DiskSpec {
	size := common.MinRootDiskSizeGiB
	if cons.RootDisk != nil && *cons.RootDisk > size {
		size = common.MiBToGiB(*cons.RootDisk)
	}
	dSpec := google.DiskSpec{
		SizeHintGB: size,
		ImageURL:   imageBasePath + spec.Image.Id,
		Boot:       true,
		AutoDelete: true,
	}
	if cons.RootDisk != nil && dSpec.TooSmall() {
		msg := "Ignoring root-disk constraint of %dM because it is smaller than the GCE image size of %dG"
		logger.Infof(msg, *cons.RootDisk, google.MinDiskSizeGB)
	}
	return []google.DiskSpec{dSpec}
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
