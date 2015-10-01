// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/lxd/lxd_client"
	"github.com/juju/juju/state/multiwatcher"
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

	series := args.Tools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", args.InstanceConfig.MachineId, series)

	if err := env.finishInstanceConfig(args); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Handle constraints?

	raw, err := env.newRawInstance(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("started instance %q", raw.Name)
	inst := newInstance(raw, env)

	// Build the result.
	hwc := env.getHardwareCharacteristics(args, inst)
	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

func (env *environ) finishInstanceConfig(args environs.StartInstanceParams) error {
	args.InstanceConfig.Tools = args.Tools[0]
	logger.Debugf("tools: %#v", args.InstanceConfig.Tools)

	args.InstanceConfig.MachineContainerType = env.ecfg.containerType()

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.ecfg.Config); err != nil {
		return errors.Trace(err)
	}

	// TODO: evaluate the impact of setting the constraints on the
	// instanceConfig for all machines rather than just state server nodes.
	// This limitation is why the constraints are assigned directly here.
	args.InstanceConfig.Constraints = args.Constraints

	args.InstanceConfig.AgentEnvironment[agent.Namespace] = env.ecfg.namespace()

	return nil
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(args environs.StartInstanceParams) (*lxd_client.Instance, error) {
	return nil, errors.NotImplementedf("")

	machineID := common.MachineFullName(env, args.InstanceConfig.MachineId)

	metadata, err := getMetadata(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	//tags := []string{
	//	env.globalFirewallName(),
	//	machineID,
	//}
	// TODO(ericsnow) Use the env ID for the network name (instead of default)?
	// TODO(ericsnow) Make the network name configurable?
	// TODO(ericsnow) Support multiple networks?
	// TODO(ericsnow) Use a different net interface name? Configurable?
	instSpec := lxd_client.InstanceSpec{
		Name: machineID,
		//Type:              spec.InstanceType.Name,
		//Disks:             getDisks(spec, args.Constraints),
		//NetworkInterfaces: []string{"ExternalNAT"},
		Metadata: metadata,
		//Tags:              tags,
		// Network is omitted (left empty).
	}

	inst, err := env.raw.AddInstance(instSpec)
	return inst, errors.Trace(err)
}

// getMetadata builds the raw "user-defined" metadata for the new
// instance (relative to the provided args) and returns it.
func getMetadata(args environs.StartInstanceParams) (map[string]string, error) {
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("LXD user data; %d bytes", len(userData))

	//b64UserData := base64.StdEncoding.EncodeToString([]byte(userData))
	//metadata := map[string]string{
	//	metadataKeyIsState: metadataValueFalse,
	//	// We store a gz snapshop of information that is used by
	//	// cloud-init and unpacked in to the /var/lib/cloud/instances folder
	//	// for the instance. Due to a limitation with GCE and binary blobs
	//	// we base64 encode the data before storing it.
	//	metadataKeyCloudInit: b64UserData,
	//	// Valid encoding values are determined by the cloudinit GCE data source.
	//	// See: http://cloudinit.readthedocs.org
	//	metadataKeyEncoding: "base64",
	//	metadataKeySSHKeys:  authKeys,
	//}
	//if isStateServer(args.InstanceConfig) {
	//	metadata[metadataKeyIsState] = metadataValueTrue
	//}
	metadata := map[string]string{}

	return metadata, nil
}

// getHardwareCharacteristics compiles hardware-related details about
// the given instance and relative to the provided spec and returns it.
func (env *environ) getHardwareCharacteristics(args environs.StartInstanceParams, inst *environInstance) *instance.HardwareCharacteristics {
	// TODO(ericsnow) Fill this in more.
	hwc := instance.HardwareCharacteristics{
	//Arch:             &spec.Image.Arch,
	//Mem:              &spec.InstanceType.Mem,
	//CpuCores:         &spec.InstanceType.CpuCores,
	//CpuPower:         spec.InstanceType.CpuPower,
	//RootDisk:         &rootDiskMB,
	//AvailabilityZone: &inst.base.ZoneName,
	// Tags: not supported in LXD?.
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
	err := env.raw.RemoveInstances(prefix, ids...)
	return errors.Trace(err)
}
