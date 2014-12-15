// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// Please note that in order to fulfil the demands made of Instances and
	// AllInstances, it is imperative that some environment feature be used to
	// keep track of which instances were actually started by juju.
	env = env.getSnapshot()

	// Start a new raw instance.

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

	inst := &environInstance{
		id:   instance.Id(raw.Name),
		env:  env,
		zone: raw.Zone,
	}
	inst.update(env, raw)
	logger.Infof("started instance %q in %q", inst.Id(), raw.Zone)

	// Handle the new instance.

	env.handleStateMachine(args, raw)

	// Build the result.

	hwc := env.getHardwareCharacteristics(spec, raw)

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
		Region:      env.gce.region,
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
		// TODO(ericsnow) Is this right?
		Storage: []string{storageScratch, storagePersistent},
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

	regionURL := env.gce.regionURL()
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{ic.Region, regionURL},
		Series:    []string{ic.Series},
		Arches:    ic.Arches,
		Stream:    stream,
	})

	matchingImages, _, err := imagemetadata.Fetch(sources, imageConstraint, signedImageDataOnly)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instanceTypes, err := env.listInstanceTypes(ic)
	if err != nil {
		return nil, errors.Trace(err)
	}

	images := instances.ImageMetadataToImages(matchingImages)
	spec, err := instances.FindInstanceSpec(images, ic, instanceTypes)
	return spec, errors.Trace(err)
}

func (env *environ) newRawInstance(args environs.StartInstanceParams, spec *instances.InstanceSpec) (*compute.Instance, error) {
	userData, err := environs.ComposeUserData(args.MachineConfig, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("GCE user data; %d bytes", len(userData))

	machineID := common.MachineFullName(env, args.MachineConfig.MachineId)
	disks := getDisks(spec, args.Constraints)
	instance := &compute.Instance{
		// TODO(ericsnow) populate/verify these values.
		Name: machineID,
		// TODO(ericsnow) The GCE instance types need to be registered.
		MachineType: spec.InstanceType.Name,
		Disks:       disks,
		// TODO(ericsnow) Do we really need this?
		Metadata: &compute.Metadata{Items: []*compute.MetadataItems{{
			Key:   "metadata.cloud-init:user-data",
			Value: string(userData),
		}}},
	}

	availabilityZones, err := env.parseAvailabilityZones(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := env.gce.newInstance(instance, availabilityZones); err != nil {
		return nil, errors.Trace(err)
	}
	return instance, nil
}

func getDisks(spec *instances.InstanceSpec, cons constraints.Value) []*compute.AttachedDisk {
	// TODO(ericsnow) Are we passing the right image value?
	rootDisk, size := diskSpec(cons.RootDisk, spec.Image.Id, true)
	if cons.RootDisk != nil && size == minDiskSize {
		msg := "Ignoring root-disk constraint of %dM because it is smaller than the GCE image size of %dM"
		logger.Infof(msg, *cons.RootDisk, minDiskSize)
	}
	return []*compute.AttachedDisk{rootDisk}
}

func (env *environ) handleStateMachine(args environs.StartInstanceParams, raw *compute.Instance) {
	if multiwatcher.AnyJobNeedsState(args.MachineConfig.Jobs...) {
		err := common.AddStateInstance(env.Storage(), instance.Id(raw.Name))
		if err != nil {
			logger.Errorf("could not record instance in provider-state: %v", err)
		}
	}
}

func (env *environ) getHardwareCharacteristics(spec *instances.InstanceSpec, raw *compute.Instance) *instance.HardwareCharacteristics {
	rawSize := raw.Disks[0].InitializeParams.DiskSizeGb
	rootDiskSize := uint64(rawSize) * 1024
	hwc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &rootDiskSize,
		// TODO(ericsnow) Add Tags here?
		// Tags *compute.Tags
		AvailabilityZone: &raw.Zone,
	}
	return &hwc
}

func (env *environ) AllInstances() ([]instance.Instance, error) {
	instances, err := env.instances()
	return instances, errors.Trace(err)
}

func (env *environ) StopInstances(instances ...instance.Id) error {
	_ = env.getSnapshot()
	return errors.Trace(errNotImplemented)
}
