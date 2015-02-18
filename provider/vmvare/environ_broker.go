// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

func isStateServer(mcfg *cloudinit.MachineConfig) bool {
	return multiwatcher.AnyJobNeedsState(mcfg.Jobs...)
}

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	env = env.getSnapshot()

	// Start a new instance.
	if args.MachineConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}

	img, err := findImageMetadata(env, args)
	if err := env.finishMachineConfig(args, img); err != nil {
		return nil, errors.Trace(err)
	}

	raw, hwc, err := newRawInstance(env, args, img)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("started instance %q", raw.Name)
	inst := newInstance(raw, env)

	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

var newRawInstance = func(env *environ, args environs.StartInstanceParams, img *imagemetadata.ImageMetadata) (*mo.VirtualMachine, *instance.HardwareCharacteristics, error) {
	return env.newRawInstance(args, img)
}

// finishMachineConfig updates args.MachineConfig in place. Setting up
// the API, StateServing, and SSHkeys information.
func (env *environ) finishMachineConfig(args environs.StartInstanceParams, img *imagemetadata.ImageMetadata) error {
	envTools, err := args.Tools.Match(tools.Filter{Arch: img.Arch})
	if err != nil {
		return err
	}

	args.MachineConfig.Tools = envTools[0]
	return environs.FinishMachineConfig(args.MachineConfig, env.Config())
}

var findImageMetadata = func(env *environ, args environs.StartInstanceParams) (*imagemetadata.ImageMetadata, error) {
	return env.findImageMetadata(args)
}

func (env *environ) findImageMetadata(args environs.StartInstanceParams) (*imagemetadata.ImageMetadata, error) {
	arches := args.Tools.Arches()
	series := args.Tools.OneSeries()
	ic := &imagemetadata.ImageConstraint{
		simplestreams.LookupParams{
			Series: []string{series},
			Arches: arches,
		},
	}
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	matchingImages, _, err := imageMetadataFetch(sources, ic, false)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(matchingImages) == 0 {
		return nil, errors.Errorf("No mathicng images found for given constraints")
	}

	return matchingImages[0], nil
}

var imageMetadataFetch = imagemetadata.Fetch

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(args environs.StartInstanceParams, img *imagemetadata.ImageMetadata) (*mo.VirtualMachine, *instance.HardwareCharacteristics, error) {
	machineID := common.MachineFullName(env, args.MachineConfig.MachineId)

	userData, err := environs.ComposeUserData(args.MachineConfig, nil)
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("Vmware user data; %d bytes", len(userData))

	rootDisk := common.MinRootDiskSizeGiB
	if args.Constraints.RootDisk != nil && *args.Constraints.RootDisk > rootDisk {
		rootDisk = common.MiBToGiB(*args.Constraints.RootDisk)
	}
	cpuCores := uint64(2)
	if args.Constraints.CpuCores != nil {
		cpuCores = *args.Constraints.CpuCores
	}
	cpuPower := uint64(2000)
	if args.Constraints.CpuPower != nil {
		cpuPower = *args.Constraints.CpuPower
	}
	mem := uint64(2000)
	if args.Constraints.Mem != nil {
		mem = *args.Constraints.Mem
	}

	instSpec := types.VirtualMachineConfigSpec{
		Name:     machineID,
		Files:    &types.VirtualMachineFileInfo{VmPathName: fmt.Sprintf("[%s]", env.client.datastore.Name())},
		NumCPUs:  int(cpuCores),
		MemoryMB: int64(mem),
		CpuAllocation: &types.ResourceAllocationInfo{
			Limit:       int64(cpuPower),
			Reservation: int64(cpuPower),
		},
	}

	inst, err := env.client.CreateInstance(instSpec, img.Id, int64(rootDisk), userData)
	hwc := &instance.HardwareCharacteristics{
		Arch:     &img.Arch,
		Mem:      &mem,
		CpuCores: &cpuCores,
		CpuPower: &cpuPower,
		RootDisk: &rootDisk,
	}
	return inst, hwc, err
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
	err := env.client.RemoveInstances(prefix, ids...)
	return errors.Trace(err)
}
