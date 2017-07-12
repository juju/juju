// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"fmt"
	"io/ioutil"
	"os"
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
	cons := args.Constraints
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

	// Download and extract the OVA file. If we're bootstrapping we use
	// a temporary directory, otherwise we cache the image for future use.
	updateProgressInterval := startInstanceUpdateProgressInterval
	if args.InstanceConfig.Bootstrap != nil {
		updateProgressInterval = bootstrapUpdateProgressInterval
	}
	updateProgress := func(message string) {
		args.StatusCallback(status.Provisioning, message, nil)
	}
	ovaDir, ovf, ovaCleanup, err := env.prepareOVA(img, args.InstanceConfig, updateProgress)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer ovaCleanup()

	createVMArgs := vsphereclient.CreateVirtualMachineParams{
		Name: vmName,
		Folder: path.Join(
			controllerFolderName(args.ControllerUUID),
			env.modelFolderName(),
		),
		OVADir:                 ovaDir,
		OVF:                    string(ovf),
		UserData:               string(userData),
		Metadata:               args.InstanceConfig.Tags,
		Constraints:            cons,
		ExternalNetwork:        externalNetwork,
		Datastore:              env.ecfg.datastore(),
		UpdateProgress:         updateProgress,
		UpdateProgressInterval: updateProgressInterval,
		Clock: clock.WallClock,
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

// prepareOVA downloads and extracts the OVA, and reads the contents of the
// .ovf file contained within it.
func (env *environ) prepareOVA(
	img *OvaFileMetadata,
	instanceConfig *instancecfg.InstanceConfig,
	updateProgress func(string),
) (ovaDir, ovf string, cleanup func(), err error) {
	fail := func(err error) (string, string, func(), error) {
		return "", "", cleanup, errors.Trace(err)
	}
	defer func() {
		if err != nil && cleanup != nil {
			cleanup()
		}
	}()

	var ovaBaseDir string
	if instanceConfig.Bootstrap != nil {
		ovaTempDir, err := ioutil.TempDir("", "juju-ova")
		if err != nil {
			return fail(errors.Trace(err))
		}
		cleanup = func() {
			if err := os.RemoveAll(ovaTempDir); err != nil {
				logger.Warningf("failed to remove temp directory: %s", err)
			}
		}
		ovaBaseDir = ovaTempDir
	} else {
		// Lock the OVA cache directory for the remainder of the
		// provisioning process. It's not enough to lock just
		// around or in downloadOVA, because we refer to the
		// contents after it returns.
		unlock, err := env.provider.ovaCacheLocker.Lock()
		if err != nil {
			return fail(errors.Annotate(err, "locking OVA cache dir"))
		}
		cleanup = unlock
		ovaBaseDir = env.provider.ovaCacheDir
	}

	ovaDir, ovfPath, err := downloadOVA(
		ovaBaseDir, instanceConfig.Series, img, updateProgress,
	)
	if err != nil {
		return fail(errors.Trace(err))
	}
	ovfBytes, err := ioutil.ReadFile(ovfPath)
	if err != nil {
		return fail(errors.Trace(err))
	}

	return ovaDir, string(ovfBytes), cleanup, nil
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
