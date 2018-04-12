// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
)

const (
	startInstanceUpdateProgressInterval = 30 * time.Second
	bootstrapUpdateProgressInterval     = 5 * time.Second
)

func controllerFolderName(controllerUUID string) string {
	return fmt.Sprintf("Juju Controller (%s)", controllerUUID)
}

func modelFolderName(modelUUID, modelName string) string {
	// We must truncate model names at 33 characters, in order to keep the
	// folder name to a maximum of 80 characters. The documentation says
	// "less than 80", but testing shows that it is in fact "no more than 80".
	//
	// See https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/vim.Folder.html:
	//   "The name to be given the new folder. An entity name must be
	//   a non-empty string of less than 80 characters. The slash (/),
	//   backslash (\) and percent (%) will be escaped using the URL
	//   syntax. For example, %2F."
	const modelNameLimit = 33
	if len(modelName) > modelNameLimit {
		modelName = modelName[:modelNameLimit]
	}
	return fmt.Sprintf("Model %q (%s)", modelName, modelUUID)
}

// vmdkDirectoryName returns the name of the datastore directory in which
// the base VMDKs are stored for the controller.
func vmdkDirectoryName(controllerUUID string) string {
	return fmt.Sprintf("juju-vmdks/%s", controllerUUID)
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(args environs.StartInstanceParams) (result *environs.StartInstanceResult, err error) {
	err = env.withSession(func(env *sessionEnviron) error {
		result, err = env.StartInstance(args)
		return err
	})
	return result, err
}

// StartInstance implements environs.InstanceBroker.
func (env *sessionEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	img, err := findImageMetadata(env, args)
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}
	if err := env.finishMachineConfig(args, img); err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	vm, hw, err := env.newRawInstance(args, img)
	if err != nil {
		args.StatusCallback(status.ProvisioningError, fmt.Sprint(err), nil)
		return nil, errors.Trace(err)
	}

	logger.Infof("started instance %q", vm.Name)
	logger.Tracef("instance data %+v", vm)
	inst := newInstance(vm, env.environ)
	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hw,
	}
	return &result, nil
}

//this variable is exported, because it has to be rewritten in external unit tests
var FinishInstanceConfig = instancecfg.FinishInstanceConfig

// finishMachineConfig updates args.MachineConfig in place. Setting up
// the API, StateServing, and SSHkeys information.
func (env *sessionEnviron) finishMachineConfig(args environs.StartInstanceParams, img *OvaFileMetadata) error {
	envTools, err := args.Tools.Match(tools.Filter{Arch: img.Arch})
	if err != nil {
		return err
	}
	if err := args.InstanceConfig.SetTools(envTools); err != nil {
		return errors.Trace(err)
	}
	return FinishInstanceConfig(args.InstanceConfig, env.Config())
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *sessionEnviron) newRawInstance(
	args environs.StartInstanceParams,
	img *OvaFileMetadata,
) (_ *mo.VirtualMachine, _ *instance.HardwareCharacteristics, err error) {

	vmName, err := env.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, nil, common.ZoneIndependentError(err)
	}

	series := args.Tools.OneSeries()
	cloudcfg, err := cloudinit.New(series)
	if err != nil {
		return nil, nil, common.ZoneIndependentError(err)
	}
	cloudcfg.AddPackage("open-vm-tools")
	cloudcfg.AddPackage("iptables-persistent")

	// Make sure the hostname is resolvable by adding it to /etc/hosts.
	cloudcfg.ManageEtcHosts(true)

	internalMac, err := vsphereclient.GenerateMAC()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	interfaces := []network.InterfaceInfo{{
		InterfaceName: "eth0",
		MACAddress:    internalMac,
		InterfaceType: network.EthernetInterface,
		ConfigType:    network.ConfigDHCP,
	}}
	networkDevices := []vsphereclient.NetworkDevice{{MAC: internalMac, Network: env.ecfg.primaryNetwork()}}

	// TODO(wpk) We need to add a firewall -AND- make sure that if it's a controller we
	// have API port open.
	externalNetwork := env.ecfg.externalNetwork()
	if externalNetwork != "" {
		externalMac, err := vsphereclient.GenerateMAC()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		interfaces = append(interfaces, network.InterfaceInfo{
			InterfaceName: "eth1",
			MACAddress:    externalMac,
			InterfaceType: network.EthernetInterface,
			ConfigType:    network.ConfigDHCP,
		})
		networkDevices = append(networkDevices, vsphereclient.NetworkDevice{MAC: externalMac, Network: externalNetwork})
	}
	// TODO(wpk) There's no (known) way to tell cloud-init to disable network (using cloudinit.CloudInitNetworkConfigDisabled)
	// so the network might be double-configured. That should be ok as long as we're using DHCP.
	err = cloudcfg.AddNetworkConfig(interfaces)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, VsphereRenderer{})
	if err != nil {
		return nil, nil, common.ZoneIndependentError(
			errors.Annotate(err, "cannot make user data"),
		)
	}
	logger.Debugf("Vmware user data; %d bytes", len(userData))

	// Obtain the final constraints by merging with defaults.
	cons := args.Constraints
	minRootDisk := common.MinRootDiskSizeGiB(args.InstanceConfig.Series) * 1024
	if cons.RootDisk == nil || *cons.RootDisk < minRootDisk {
		cons.RootDisk = &minRootDisk
	}

	// Download and extract the OVA file. If we're bootstrapping we use
	// a temporary directory, otherwise we cache the image for future use.
	updateProgressInterval := startInstanceUpdateProgressInterval
	if args.InstanceConfig.Bootstrap != nil {
		updateProgressInterval = bootstrapUpdateProgressInterval
	}
	updateProgress := func(message string) {
		args.StatusCallback(status.Provisioning, message, nil)
	}

	readOVA := func() (string, io.ReadCloser, error) {
		resp, err := http.Get(img.URL)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		return img.URL, resp.Body, nil
	}

	createVMArgs := vsphereclient.CreateVirtualMachineParams{
		Name: vmName,
		Folder: path.Join(
			controllerFolderName(args.ControllerUUID),
			env.modelFolderName(),
		),
		Series:                 series,
		ReadOVA:                readOVA,
		OVASHA256:              img.Sha256,
		VMDKDirectory:          vmdkDirectoryName(args.ControllerUUID),
		UserData:               string(userData),
		Metadata:               args.InstanceConfig.Tags,
		Constraints:            cons,
		NetworkDevices:         networkDevices,
		Datastore:              env.ecfg.datastore(),
		UpdateProgress:         updateProgress,
		UpdateProgressInterval: updateProgressInterval,
		Clock: clock.WallClock,
	}

	// Attempt to create a VM in each of the AZs in turn.
	logger.Debugf("attempting to create VM in availability zone %s", args.AvailabilityZone)
	availZone, err := env.availZone(args.AvailabilityZone)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	createVMArgs.ComputeResource = &availZone.(*vmwareAvailZone).r

	vm, err := env.client.CreateVirtualMachine(env.ctx, createVMArgs)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	hw := &instance.HardwareCharacteristics{
		Arch:     &img.Arch,
		Mem:      cons.Mem,
		CpuCores: cons.CpuCores,
		CpuPower: cons.CpuPower,
		RootDisk: cons.RootDisk,
	}
	return vm, hw, err
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances() (instances []instance.Instance, err error) {
	err = env.withSession(func(env *sessionEnviron) error {
		instances, err = env.AllInstances()
		return err
	})
	return instances, err
}

// AllInstances implements environs.InstanceBroker.
func (env *sessionEnviron) AllInstances() ([]instance.Instance, error) {
	modelFolderPath := path.Join(
		controllerFolderName("*"),
		env.modelFolderName(),
	)
	vms, err := env.client.VirtualMachines(env.ctx, modelFolderPath+"/*")
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Turn mo.VirtualMachine values into *environInstance values,
	// whether or not we got an error.
	results := make([]instance.Instance, len(vms))
	for i, vm := range vms {
		results[i] = newInstance(vm, env.environ)
	}
	return results, err
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(ids ...instance.Id) error {
	return env.withSession(func(env *sessionEnviron) error {
		return env.StopInstances(ids...)
	})
}

// StopInstances implements environs.InstanceBroker.
func (env *sessionEnviron) StopInstances(ids ...instance.Id) error {
	modelFolderPath := path.Join(
		controllerFolderName("*"),
		env.modelFolderName(),
	)
	results := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id instance.Id) {
			defer wg.Done()
			results[i] = env.client.RemoveVirtualMachines(
				env.ctx,
				path.Join(modelFolderPath, string(id)),
			)
		}(i, id)
	}
	wg.Wait()

	var errIds []instance.Id
	var errs []error
	for i, err := range results {
		if err != nil {
			errIds = append(errIds, ids[i])
			errs = append(errs, err)
		}
	}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.Annotatef(errs[0], "failed to stop instance %s", errIds[0])
	default:
		return errors.Errorf(
			"failed to stop instances %s: %s",
			errIds, errs,
		)
	}
}
