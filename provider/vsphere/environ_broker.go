// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
)

const (
	DefaultCpuCores = uint64(2)
	DefaultCpuPower = uint64(2000)
	DefaultMemMb    = uint64(2000)
)

const (
	metadataKeyIsController     = "juju_is_controller_key"
	metadataValueIsController   = "juju_is_controller_value"
	metadataKeyControllerUUID   = "juju_controller_uuid_key"
	metadataValueControllerUUID = "juju_controller_uuid_value"
)

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
		return nil, errors.Trace(err)
	}
	if err := env.finishMachineConfig(args, img); err != nil {
		return nil, errors.Trace(err)
	}

	vm, hw, err := env.newRawInstance(args, img)
	if err != nil {
		args.StatusCallback(status.ProvisioningError, fmt.Sprint(err), nil)
		return nil, errors.Trace(err)
	}

	logger.Infof("started instance %q", vm.Name)
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
) (*mo.VirtualMachine, *instance.HardwareCharacteristics, error) {
	vmName, err := env.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	cloudcfg, err := cloudinit.New(args.Tools.OneSeries())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	cloudcfg.AddPackage("open-vm-tools")
	cloudcfg.AddPackage("iptables-persistent")

	// Make sure the hostname is resolvable by adding it to /etc/hosts.
	cloudcfg.ManageEtcHosts(true)

	// If an "external network" is specified, add the boot commands
	// necessary to configure it.
	externalNetwork := env.ecfg.externalNetwork()
	if externalNetwork != "" {
		apiPort := 0
		if args.InstanceConfig.Controller != nil {
			apiPort = args.InstanceConfig.Controller.Config.APIPort()
		}
		commands := common.ConfigureExternalIpAddressCommands(apiPort)
		cloudcfg.AddBootCmd(commands...)
	}

	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, VsphereRenderer{})
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("Vmware user data; %d bytes", len(userData))

	// Obtain the final constraints by merging with defaults.
	uint64ptr := func(v uint64) *uint64 {
		return &v
	}
	defaultCons := constraints.Value{
		CpuCores: uint64ptr(DefaultCpuCores),
		CpuPower: uint64ptr(DefaultCpuPower),
		Mem:      uint64ptr(DefaultMemMb),
	}
	validator, err := env.ConstraintsValidator()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	cons, err := validator.Merge(defaultCons, args.Constraints)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	minRootDisk := common.MinRootDiskSizeGiB(args.InstanceConfig.Series) * 1024
	if cons.RootDisk == nil || *cons.RootDisk < minRootDisk {
		cons.RootDisk = &minRootDisk
	}

	// Identify which zones may be used, taking into
	// account placement directives.
	zones, err := env.parseAvailabilityZones(args)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Download and extract the OVA file.
	args.StatusCallback(status.Provisioning, fmt.Sprintf("downloading %s", img.URL), nil)
	ovaDir, err := ioutil.TempDir("", "juju-ova")
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer func() {
		if err := os.RemoveAll(ovaDir); err != nil {
			logger.Warningf("failed to remove temp directory: %s", err)
		}
	}()
	ovf, err := downloadOva(ovaDir, img.URL)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	metadata := map[string]string{
		metadataKeyControllerUUID: args.ControllerUUID,
	}
	if args.InstanceConfig.Controller != nil {
		metadata[metadataKeyIsController] = metadataValueIsController
	}

	createVMArgs := vsphereclient.CreateVirtualMachineParams{
		Name:            vmName,
		OVADir:          ovaDir,
		OVF:             ovf,
		UserData:        string(userData),
		Metadata:        metadata,
		Constraints:     cons,
		ExternalNetwork: externalNetwork,
		UpdateProgress: func(message string) {
			args.StatusCallback(status.Provisioning, message, nil)
		},
	}

	// Attempt to create a VM in each of the AZs in turn.
	var vm *mo.VirtualMachine
	var lastError error
	for _, zone := range zones {
		logger.Debugf("attempting to create VM in availability zone %s", zone)
		availZone, err := env.availZone(zone)
		if err != nil {
			logger.Warningf("failed to get availability zone %s: %s", zone, err)
			lastError = err
			continue
		}
		createVMArgs.ComputeResource = &availZone.(*vmwareAvailZone).r

		vm, err = env.client.CreateVirtualMachine(env.ctx, createVMArgs)
		if err != nil {
			logger.Warningf("failed to create instance in availability zone %s: %s", zone, err)
			lastError = err
			continue
		}
		lastError = nil
		break
	}
	if lastError != nil {
		return nil, nil, errors.Annotate(lastError, "failed to create instance in any availability zone")
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
	prefix := env.namespace.Prefix()
	vms, err := env.client.VirtualMachines(env.ctx, prefix+"*")
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
	results := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id instance.Id) {
			defer wg.Done()
			results[i] = env.client.RemoveVirtualMachines(env.ctx, string(id))
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
