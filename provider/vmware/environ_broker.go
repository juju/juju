// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware

import (
	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"

	coreCloudinit "github.com/juju/juju/cloudinit"
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
	if err != nil {
		return nil, errors.Trace(err)
	}
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

var newRawInstance = func(env *environ, args environs.StartInstanceParams, img *OvfFileMetadata) (*mo.VirtualMachine, *instance.HardwareCharacteristics, error) {
	return env.newRawInstance(args, img)
}

// finishMachineConfig updates args.MachineConfig in place. Setting up
// the API, StateServing, and SSHkeys information.
func (env *environ) finishMachineConfig(args environs.StartInstanceParams, img *OvfFileMetadata) error {
	envTools, err := args.Tools.Match(tools.Filter{Arch: img.Arch})
	if err != nil {
		return err
	}

	args.MachineConfig.Tools = envTools[0]
	return environs.FinishMachineConfig(args.MachineConfig, env.Config())
}

type OvfFileMetadata struct {
	Url      string
	Arch     string `json:"arch"`
	Size     int    `json:"size"`
	Path     string `json:"path"`
	FileType string `json:"ftype"`
	Sha256   string `json:"sha256"`
	Md5      string `json:"md5"`
}

func init() {
	simplestreams.RegisterStructTags(OvfFileMetadata{})
}

var findImageMetadata = func(env *environ, args environs.StartInstanceParams) (*OvfFileMetadata, error) {
	return env.findImageMetadata(args)
}

func (env *environ) findImageMetadata(args environs.StartInstanceParams) (*OvfFileMetadata, error) {
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

	matchingImages, err := imageMetadataFetch(sources, ic)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(matchingImages) == 0 {
		return nil, errors.Errorf("No mathicng images found for given constraints: %v", ic)
	}

	return matchingImages[0], nil
}

var imageMetadataFetch = func(sources []simplestreams.DataSource, cons *imagemetadata.ImageConstraint) ([]*OvfFileMetadata, error) {
	params := simplestreams.GetMetadataParams{
		StreamsVersion:   imagemetadata.StreamsVersionV1,
		OnlySigned:       false,
		LookupConstraint: cons,
		ValueParams: simplestreams.ValueParams{
			DataType:      "image-downloads",
			FilterFunc:    appendMatchingFunc,
			ValueTemplate: OvfFileMetadata{},
		},
	}
	items, _, err := simplestreams.GetMetadata(sources, params)
	if err != nil {
		return nil, err
	}
	metadata := make([]*OvfFileMetadata, len(items))
	for i, md := range items {
		metadata[i] = md.(*OvfFileMetadata)
	}
	return metadata, nil
}

var appendMatchingFunc = func(source simplestreams.DataSource, matchingImages []interface{},
	images map[string]interface{}, cons simplestreams.LookupConstraint) []interface{} {

	for _, val := range images {
		file := val.(*OvfFileMetadata)
		if file.FileType == "ovf" {
			//ignore error for url data source
			url, _ := source.URL(file.Path)
			file.Url = url
			matchingImages = append(matchingImages, file)
		}
	}
	return matchingImages
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(args environs.StartInstanceParams, img *OvfFileMetadata) (*mo.VirtualMachine, *instance.HardwareCharacteristics, error) {
	machineID := common.MachineFullName(env, args.MachineConfig.MachineId)

	config := coreCloudinit.New()
	config.SetAptUpdate(true)
	config.SetAptUpgrade(true)
	config.AddPackage("open-vm-tools")
	userData, err := environs.ComposeUserData(args.MachineConfig, config)
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

	hwc := &instance.HardwareCharacteristics{
		Arch:     &img.Arch,
		Mem:      &mem,
		CpuCores: &cpuCores,
		CpuPower: &cpuPower,
		RootDisk: &rootDisk,
	}
	inst, err := env.client.CreateInstance(machineID, hwc, img, userData, args.MachineConfig.AuthorizedKeys)
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
