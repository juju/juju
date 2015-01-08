// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"encoding/base64"
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/gceapi"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

func isStateServer(mcfg *cloudinit.MachineConfig) bool {
	return multiwatcher.AnyJobNeedsState(mcfg.Jobs...)
}

func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// Please note that in order to fulfil the demands made of Instances and
	// AllInstances, it is imperative that some environment feature be used to
	// keep track of which instances were actually started by juju.
	env = env.getSnapshot()

	// Start a new instance.

	if args.MachineConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}

	spec, err := env.finishMachineConfig(args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	raw, err := env.newRawInstance(args, spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("started instance %q in zone %q", raw.ID, raw.Zone)
	inst := newInstance(raw, env)

	// Open API port on state server.
	if isStateServer(args.MachineConfig) {
		ports := []network.PortRange{{
			FromPort: args.MachineConfig.StateServingInfo.APIPort,
			ToPort:   args.MachineConfig.StateServingInfo.APIPort,
			Protocol: "tcp",
		}}
		if err := env.gce.OpenPorts(raw.ID, ports); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Build the result.
	hwc := env.getHardwareCharacteristics(spec, inst)
	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

func (env *environ) finishMachineConfig(args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	arches := args.Tools.Arches()
	series := args.Tools.OneSeries()
	spec, err := env.findInstanceSpec(env.Config().ImageStream(), &instances.InstanceConstraint{
		Region:      env.ecfg.region(),
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	envTools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, errors.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	args.MachineConfig.Tools = envTools[0]
	err = environs.FinishMachineConfig(args.MachineConfig, env.Config())
	return spec, errors.Trace(err)
}

func (env *environ) findInstanceSpec(stream string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloudSpec, err := env.cloudSpec(ic.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}

	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
		Series:    []string{ic.Series},
		Arches:    ic.Arches,
		Stream:    stream,
	})

	matchingImages, _, err := imagemetadata.Fetch(sources, imageConstraint, signedImageDataOnly)
	if err != nil {
		return nil, errors.Trace(err)
	}

	images := instances.ImageMetadataToImages(matchingImages)
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	return spec, errors.Trace(err)
}

func (env *environ) newRawInstance(args environs.StartInstanceParams, spec *instances.InstanceSpec) (*gceapi.Instance, error) {
	machineID := common.MachineFullName(env, args.MachineConfig.MachineId)

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
	instSpec := gceapi.InstanceSpec{
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

	inst, err := instSpec.Create(env.gce, zones)
	return inst, errors.Trace(err)
}

func getMetadata(args environs.StartInstanceParams) (map[string]string, error) {
	userData, err := environs.ComposeUserData(args.MachineConfig, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("GCE user data; %d bytes", len(userData))

	authKeys, err := gceapi.FormatAuthorizedKeys(args.MachineConfig.AuthorizedKeys, "ubuntu")
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
	if isStateServer(args.MachineConfig) {
		metadata[metadataKeyIsState] = metadataValueTrue
	}

	return metadata, nil
}

func getDisks(spec *instances.InstanceSpec, cons constraints.Value) []gceapi.DiskSpec {
	size := common.MinRootDiskSizeGiB
	if cons.RootDisk != nil && *cons.RootDisk > uint64(size) {
		size = int64(common.MiBToGiB(*cons.RootDisk))
	}
	dSpec := gceapi.DiskSpec{
		SizeHintGB: size,
		ImageURL:   imageBasePath + spec.Image.Id,
		Boot:       true,
		AutoDelete: true,
	}
	if cons.RootDisk != nil && dSpec.TooSmall() {
		msg := "Ignoring root-disk constraint of %dM because it is smaller than the GCE image size of %dG"
		logger.Infof(msg, *cons.RootDisk, gceapi.MinDiskSizeGB)
	}
	return []gceapi.DiskSpec{dSpec}
}

func (env *environ) getHardwareCharacteristics(spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	rootDiskMB := uint64(inst.base.RootDiskGB()) * 1024
	hwc := instance.HardwareCharacteristics{
		Arch:             &spec.Image.Arch,
		Mem:              &spec.InstanceType.Mem,
		CpuCores:         &spec.InstanceType.CpuCores,
		CpuPower:         spec.InstanceType.CpuPower,
		RootDisk:         &rootDiskMB,
		AvailabilityZone: &inst.base.Zone,
		// Tags: not supported in GCE.
	}
	return &hwc
}

func (env *environ) AllInstances() ([]instance.Instance, error) {
	instances, err := env.instances()
	return instances, errors.Trace(err)
}

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
