// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/govmomi/vim25/mo"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
	"github.com/juju/utils"
)

const (
	DefaultCpuCores = uint64(2)
	DefaultCpuPower = uint64(2000)
	DefaultMemMb    = uint64(2000)
)

func isStateServer(mcfg *instancecfg.InstanceConfig) bool {
	return multiwatcher.AnyJobNeedsState(mcfg.Jobs...)
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	env = env.getSnapshot()

	if args.InstanceConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}

	img, err := findImageMetadata(env, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := env.finishMachineConfig(args, img); err != nil {
		return nil, errors.Trace(err)
	}

	raw, hwc, err := env.newRawInstance(args, img)
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

//this variable is exported, because it has to be rewritten in external unit tests
var FinishInstanceConfig = instancecfg.FinishInstanceConfig

// finishMachineConfig updates args.MachineConfig in place. Setting up
// the API, StateServing, and SSHkeys information.
func (env *environ) finishMachineConfig(args environs.StartInstanceParams, img *OvaFileMetadata) error {
	envTools, err := args.Tools.Match(tools.Filter{Arch: img.Arch})
	if err != nil {
		return err
	}

	args.InstanceConfig.Tools = envTools[0]
	return FinishInstanceConfig(args.InstanceConfig, env.Config())
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(args environs.StartInstanceParams, img *OvaFileMetadata) (*mo.VirtualMachine, *instance.HardwareCharacteristics, error) {
	machineID := common.MachineFullName(env, args.InstanceConfig.MachineId)

	cloudcfg, err := cloudinit.New(args.Tools.OneSeries())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	cloudcfg.AddPackage("open-vm-tools")
	cloudcfg.AddPackage("iptables-persistent")
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg)
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot make user data")
	}
	userData, err = utils.Gunzip(userData)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	logger.Debugf("Vmware user data; %d bytes", len(userData))

	rootDisk := common.MinRootDiskSizeGiB * 1024
	if args.Constraints.RootDisk != nil && *args.Constraints.RootDisk > rootDisk {
		rootDisk = *args.Constraints.RootDisk
	}
	cpuCores := DefaultCpuCores
	if args.Constraints.CpuCores != nil {
		cpuCores = *args.Constraints.CpuCores
	}
	cpuPower := DefaultCpuPower
	if args.Constraints.CpuPower != nil {
		cpuPower = *args.Constraints.CpuPower
	}
	mem := DefaultMemMb
	if args.Constraints.Mem != nil {
		mem = *args.Constraints.Mem
	}

	hwc := &instance.HardwareCharacteristics{
		Arch:     &img.Arch,
		Mem:      &mem,
		CpuCores: &cpuCores,
		CpuPower: &cpuPower,
		RootDisk: &rootDisk,
	}
	zones, err := env.parseAvailabilityZones(args)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	var inst *mo.VirtualMachine
	for _, zone := range zones {
		var availZone *vmwareAvailZone
		availZone, err = env.availZone(zone)
		if err != nil {
			logger.Warningf("Error while getting availability zone %s: %s", zone, err)
			continue
		}
		apiPort := 0
		if isStateServer(args.InstanceConfig) {
			apiPort = args.InstanceConfig.StateServingInfo.APIPort
		}
		spec := &instanceSpec{
			machineID: machineID,
			zone:      availZone,
			hwc:       hwc,
			img:       img,
			userData:  userData,
			sshKey:    args.InstanceConfig.AuthorizedKeys,
			isState:   isStateServer(args.InstanceConfig),
			apiPort:   apiPort,
		}
		inst, err = env.client.CreateInstance(env.ecfg, spec)
		if err != nil {
			logger.Warningf("Error while trying to create instance in %s availability zone: %s", zone, err)
			continue
		}
		break
	}
	if err != nil {
		return nil, nil, errors.Annotate(err, "Can't create instance in any of availability zones, last error")
	}
	return inst, hwc, err
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances() ([]instance.Instance, error) {
	instances, err := env.instances()
	return instances, errors.Trace(err)
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(instances ...instance.Id) error {
	env = env.getSnapshot()

	var ids []string
	for _, id := range instances {
		ids = append(ids, string(id))
	}

	err := env.client.RemoveInstances(ids...)
	return errors.Trace(err)
}
