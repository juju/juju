// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/gce/google"
)

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(ctx context.Context, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// Start a new instance.

	spec, err := buildInstanceSpec(env, ctx, args)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	if err := env.finishInstanceConfig(args, spec); err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	// Validate availability zone.
	volumeAttachmentsZone, err := volumeAttachmentsZone(args.VolumeAttachments)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	if err := validateAvailabilityZoneConsistency(args.AvailabilityZone, volumeAttachmentsZone); err != nil {
		return nil, errors.Trace(err)
	}

	raw, err := newRawInstance(env, ctx, args, spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof(ctx, "started instance %q in zone %q", raw.ID, raw.ZoneName)
	inst := newInstance(raw, env)

	// Build the result.
	hwc := getHardwareCharacteristics(env, spec, inst)
	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

var buildInstanceSpec = func(env *environ, ctx context.Context, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	return env.buildInstanceSpec(ctx, args)
}

var newRawInstance = func(env *environ, ctx context.Context, args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	return env.newRawInstance(ctx, args, spec)
}

var getHardwareCharacteristics = func(env *environ, spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	return env.getHardwareCharacteristics(spec, inst)
}

// finishInstanceConfig updates args.InstanceConfig in place. Setting up
// the API, StateServing, and SSHkeys information.
func (env *environ) finishInstanceConfig(args environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	if err := args.InstanceConfig.SetTools(args.Tools); err != nil {
		return errors.Trace(err)
	}
	return instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config())
}

// buildInstanceSpec builds an instance spec from the provided args
// and returns it. This includes pulling the simplestreams data for the
// machine type, region, and other constraints.
func (env *environ) buildInstanceSpec(ctx context.Context, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	instTypesAndCosts, err := env.InstanceTypes(ctx, constraints.Value{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	arch, err := args.Tools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec, err := findInstanceSpec(
		env, &instances.InstanceConstraint{
			Region:      env.cloud.Region,
			Base:        args.InstanceConfig.Base,
			Arch:        arch,
			Constraints: args.Constraints,
		},
		args.ImageMetadata,
		instTypesAndCosts.InstanceTypes,
	)
	return spec, errors.Trace(err)
}

var findInstanceSpec = func(
	env *environ,
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
	allInstanceTypes []instances.InstanceType,
) (*instances.InstanceSpec, error) {
	return env.findInstanceSpec(ic, imageMetadata, allInstanceTypes)
}

// findInstanceSpec initializes a new instance spec for the given
// constraints and returns it. This only covers populating the
// initial data for the spec.
func (env *environ) findInstanceSpec(
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
	allInstanceTypes []instances.InstanceType,
) (*instances.InstanceSpec, error) {
	images := instances.ImageMetadataToImages(imageMetadata)
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	return spec, errors.Trace(err)
}

func (env *environ) imageURLBase(os ostype.OSType) (string, error) {
	base, useCustomPath := env.ecfg.baseImagePath()
	if useCustomPath {
		return base, nil
	}

	switch os {
	case ostype.Ubuntu:
		switch env.Config().ImageStream() {
		case "daily":
			base = ubuntuDailyImageBasePath
		case "pro":
			base = ubuntuProImageBasePath
		default:
			base = ubuntuImageBasePath
		}
	default:
		return "", errors.Errorf("os %s is not supported on the gce provider", os.String())
	}

	return base, nil
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(
	ctx context.Context, args environs.StartInstanceParams, spec *instances.InstanceSpec,
) (_ *google.Instance, err error) {
	hostname, err := env.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	os := ostype.OSTypeForName(args.InstanceConfig.Base.OS)
	metadata, err := getMetadata(args, os)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	tags := []string{
		env.globalFirewallName(),
		hostname,
	}

	imageURLBase, err := env.imageURLBase(os)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	disks, err := getDisks(
		spec, args.Constraints,
		os,
		env.Config().UUID(),
		imageURLBase,
	)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	allocatePublicIP := true
	if args.Constraints.HasAllocatePublicIP() {
		allocatePublicIP = *args.Constraints.AllocatePublicIP
	}

	instArg := google.InstanceSpec{
		ID:                hostname,
		Type:              spec.InstanceType.Name,
		Disks:             disks,
		NetworkInterfaces: []string{"ExternalNAT"},
		Metadata:          metadata,
		Tags:              tags,
		AvailabilityZone:  args.AvailabilityZone,
		AllocatePublicIP:  allocatePublicIP,
	}
	serviceAccount, err := env.gce.DefaultServiceAccount()
	if err != nil {
		return nil, env.HandleCredentialError(ctx, errors.Trace(err))
	}
	logger.Debugf(ctx, "using project service account: %s", serviceAccount)

	instArg.DefaultServiceAccount = serviceAccount
	inst, err := env.gce.AddInstance(instArg)
	if err != nil {
		// We currently treat all AddInstance failures
		// as being zone-specific, so we'll retry in
		// another zone.
		return nil, env.HandleCredentialError(ctx, err)
	}

	return inst, nil
}

// getMetadata builds the raw "user-defined" metadata for the new
// instance (relative to the provided args) and returns it.
func getMetadata(args environs.StartInstanceParams, os ostype.OSType) (map[string]string, error) {
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, nil, GCERenderer{})
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf(context.TODO(), "GCE user data; %d bytes", len(userData))

	metadata := make(map[string]string)
	for tag, value := range args.InstanceConfig.Tags {
		metadata[tag] = value
	}
	switch os {
	case ostype.Ubuntu:
		// We store a gz snapshop of information that is used by
		// cloud-init and unpacked in to the /var/lib/cloud/instances folder
		// for the instance. Due to a limitation with GCE and binary blobs
		// we base64 encode the data before storing it.
		metadata[metadataKeyCloudInit] = string(userData)
		// Valid encoding values are determined by the cloudinit GCE data source.
		// See: http://cloudinit.readthedocs.org
		metadata[metadataKeyEncoding] = "base64"

	default:
		return nil, errors.Errorf("cannot pack metadata for os %s on the gce provider", os.String())
	}

	return metadata, nil
}

// getDisks builds the raw spec for the disks that should be attached to
// the new instances and returns it. This will always include a root
// disk with characteristics determined by the provides args and
// constraints.
func getDisks(spec *instances.InstanceSpec, cons constraints.Value, os ostype.OSType, eUUID string, imageURLBase string) ([]google.DiskSpec, error) {
	size := common.MinRootDiskSizeGiB(os)
	if cons.RootDisk != nil && *cons.RootDisk > size {
		size = common.MiBToGiB(*cons.RootDisk)
	}
	if imageURLBase == "" {
		return nil, errors.NotValidf("imageURLBase must be set")
	}
	imageURL := imageURLBase + spec.Image.Id
	logger.Infof(context.TODO(), "fetching disk image from %v", imageURL)
	dSpec := google.DiskSpec{
		OS:         strings.ToLower(os.String()),
		SizeHintGB: size,
		ImageURL:   imageURL,
		Boot:       true,
		AutoDelete: true,
	}
	if cons.RootDisk != nil && dSpec.TooSmall() {
		msg := "Ignoring root-disk constraint of %dM because it is smaller than the GCE image size of %dG"
		logger.Infof(context.TODO(), msg, *cons.RootDisk, google.MinDiskSizeGB)
	}
	return []google.DiskSpec{dSpec}, nil
}

// getHardwareCharacteristics compiles hardware-related details about
// the given instance and relative to the provided spec and returns it.
func (env *environ) getHardwareCharacteristics(spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	rootDiskMB := inst.base.RootDiskGB() * 1024
	hwc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &rootDiskMB,
		// Tags: not supported in GCE.
	}
	if inst.base.ZoneName != "" {
		hwc.AvailabilityZone = &inst.base.ZoneName
	}
	return &hwc
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	// We want all statuses here except for "terminated" - these instances are truly dead to us.
	// According to https://cloud.google.com/compute/docs/instances/instance-life-cycle
	// there are now only "provisioning", "staging", "running", "stopping" and "terminated" states.
	// The others might have been needed for older versions of gce... Keeping here for potential
	// backward compatibility.
	nonLiveStatuses := []string{
		google.StatusDone,
		google.StatusDown,
		google.StatusProvisioning,
		google.StatusStopped,
		google.StatusStopping,
		google.StatusUp,
	}
	filters := append(instStatuses, nonLiveStatuses...)
	instances, err := getInstances(env, ctx, filters...)
	return instances, errors.Trace(err)
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *environ) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	instances, err := getInstances(env, ctx)
	return instances, errors.Trace(err)
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(ctx context.Context, instances ...instance.Id) error {
	var ids []string
	for _, id := range instances {
		ids = append(ids, string(id))
	}

	prefix := env.namespace.Prefix()
	err := env.gce.RemoveInstances(prefix, ids...)
	return env.HandleCredentialError(ctx, err)
}
