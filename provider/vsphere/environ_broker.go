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

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
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

// templateDirectoryName returns the name of the datastore directory in which
// the VM templates are stored for the controller.
func templateDirectoryName(controllerFolderName string) string {
	return path.Join(controllerFolderName, "templates")
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (result *environs.StartInstanceResult, err error) {
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		result, err = env.StartInstance(ctx, args)
		return err
	})
	return result, err
}

// StartInstance implements environs.InstanceBroker.
func (env *sessionEnviron) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	img, err := findImageMetadata(env, args)
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}
	if err := env.finishMachineConfig(args, img); err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	vm, hw, err := env.newRawInstance(ctx, args, img)
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

// FinishInstanceConfig is exported, because it has to be rewritten in external unit tests
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
	ctx context.ProviderCallContext,
	args environs.StartInstanceParams,
	img *OvaFileMetadata,
) (_ *mo.VirtualMachine, _ *instance.HardwareCharacteristics, err error) {
	if args.AvailabilityZone == "" {
		return nil, nil, errors.NotValidf("empty available zone")
	}

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

	interfaces := []corenetwork.InterfaceInfo{{
		InterfaceName: "eth0",
		MACAddress:    internalMac,
		InterfaceType: corenetwork.EthernetInterface,
		ConfigType:    corenetwork.ConfigDHCP,
		Origin:        corenetwork.OriginProvider,
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
		interfaces = append(interfaces, corenetwork.InterfaceInfo{
			InterfaceName: "eth1",
			MACAddress:    externalMac,
			InterfaceType: corenetwork.EthernetInterface,
			ConfigType:    corenetwork.ConfigDHCP,
			Origin:        corenetwork.OriginProvider,
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
	defaultDatastore := env.ecfg.datastore()
	if (cons.RootDiskSource == nil || *cons.RootDiskSource == "") && defaultDatastore != "" {
		cons.RootDiskSource = &defaultDatastore
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
		Name:                   vmName,
		Folder:                 path.Join(env.getVMFolder(), controllerFolderName(args.ControllerUUID), env.modelFolderName()),
		RootVMFolder:           env.getVMFolder(),
		Series:                 series,
		ReadOVA:                readOVA,
		OVASHA256:              img.Sha256,
		VMDKDirectory:          templateDirectoryName(controllerFolderName(args.ControllerUUID)),
		UserData:               string(userData),
		Metadata:               args.InstanceConfig.Tags,
		Constraints:            cons,
		NetworkDevices:         networkDevices,
		UpdateProgress:         updateProgress,
		UpdateProgressInterval: updateProgressInterval,
		Clock:                  clock.WallClock,
		EnableDiskUUID:         env.ecfg.enableDiskUUID(),
		IsBootstrap:            args.InstanceConfig.Bootstrap != nil,
	}

	// Attempt to create a VM in each of the AZs in turn.
	logger.Debugf("attempting to create VM in availability zone %q", args.AvailabilityZone)
	availZone, err := env.availZone(ctx, args.AvailabilityZone)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	createVMArgs.ComputeResource = &availZone.r
	createVMArgs.ResourcePool = availZone.pool.Reference()

	vm, err := env.client.CreateVirtualMachine(env.ctx, createVMArgs)
	if vsphereclient.IsExtendDiskError(err) {
		// Ensure we don't try to make the same extension across
		// different resource groups.
		err = common.ZoneIndependentError(err)
	}
	if err != nil {
		HandleCredentialError(err, env, ctx)
		return nil, nil, errors.Trace(err)

	}

	hw := &instance.HardwareCharacteristics{
		Arch:           &img.Arch,
		Mem:            cons.Mem,
		CpuCores:       cons.CpuCores,
		CpuPower:       cons.CpuPower,
		RootDisk:       cons.RootDisk,
		RootDiskSource: cons.RootDiskSource,
	}
	return vm, hw, err
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances(ctx context.ProviderCallContext) (instances []instances.Instance, err error) {
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		instances, err = env.AllInstances(ctx)
		return err
	})
	return instances, err
}

// AllInstances implements environs.InstanceBroker.
func (env *sessionEnviron) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	modelFolderPath := path.Join(env.getVMFolder(), controllerFolderName("*"), env.modelFolderName())
	vms, err := env.client.VirtualMachines(env.ctx, modelFolderPath+"/*")
	if err != nil {
		HandleCredentialError(err, env, ctx)
		return nil, errors.Trace(err)
	}

	var results []instances.Instance
	for _, vm := range vms {
		results = append(results, newInstance(vm, env.environ))
	}
	return results, err
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *environ) AllRunningInstances(ctx context.ProviderCallContext) (instances []instances.Instance, err error) {
	// AllInstances() already handles all instances irrespective of the state, so
	// here 'all' is also 'all running'.
	return env.AllInstances(ctx)
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *sessionEnviron) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	// AllInstances() already handles all instances irrespective of the state, so
	// here 'all' is also 'all running'.
	return env.AllInstances(ctx)
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.StopInstances(ctx, ids...)
	})
}

// StopInstances implements environs.InstanceBroker.
func (env *sessionEnviron) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	modelFolderPath := path.Join(env.getVMFolder(), controllerFolderName("*"), env.modelFolderName())
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
			HandleCredentialError(results[i], env, ctx)
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
