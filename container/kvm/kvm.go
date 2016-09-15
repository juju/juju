// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
)

var (
	logger = loggo.GetLogger("juju.container.kvm")

	KvmObjectFactory ContainerFactory = &containerFactory{}

	// In order for Juju to be able to create the hardware characteristics of
	// the kvm machines it creates, we need to be explicit in our definition
	// of memory, cores and root-disk.  The defaults here have been
	// extracted from the uvt-kvm executable.
	DefaultMemory uint64 = 512 // MB
	DefaultCpu    uint64 = 1
	DefaultDisk   uint64 = 8 // GB

	// There are some values where it doesn't make sense to go below.
	MinMemory uint64 = 512 // MB
	MinCpu    uint64 = 1
	MinDisk   uint64 = 2 // GB
)

// Utilized to provide a hard-coded path to kvm-ok
var kvmPath = "/usr/sbin"

// IsKVMSupported calls into the kvm-ok executable from the cpu-checkers package.
// It is a variable to allow us to overrid behaviour in the tests.
var IsKVMSupported = func() (bool, error) {

	// Prefer the user's $PATH first, but check /usr/sbin if we can't
	// find kvm-ok there
	var foundPath string
	const binName = "kvm-ok"
	if path, err := exec.LookPath(binName); err == nil {
		foundPath = path
	} else if path, err := exec.LookPath(filepath.Join(kvmPath, binName)); err == nil {
		foundPath = path
	} else {
		return false, errors.NotFoundf("%s executable", binName)
	}

	command := exec.Command(foundPath)
	output, err := command.CombinedOutput()

	if err != nil {
		return false, errors.Annotate(err, string(output))
	}
	logger.Debugf("%s output:\n%s", binName, output)
	return command.ProcessState.Success(), nil
}

// NewContainerManager returns a manager object that can start and stop kvm
// containers.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	modelUUID := conf.PopValue(container.ConfigModelUUID)
	if modelUUID == "" {
		return nil, errors.Errorf("model UUID is required")
	}
	namespace, err := instance.NewNamespace(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logDir := conf.PopValue(container.ConfigLogDir)
	if logDir == "" {
		logDir = agent.DefaultPaths.LogDir
	}
	conf.WarnAboutUnused()
	return &containerManager{namespace: namespace, logdir: logDir}, nil
}

// containerManager handles all of the business logic at the juju specific
// level. It makes sure that the necessary directories are in place, that the
// user-data is written out in the right place.
type containerManager struct {
	namespace instance.Namespace
	logdir    string
}

var _ container.Manager = (*containerManager)(nil)

// Namespace implements container.Manager.
func (manager *containerManager) Namespace() instance.Namespace {
	return manager.namespace
}

// Exposed so tests can observe our side-effects
var startParams StartParams

func (manager *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback container.StatusCallback,
) (_ instance.Instance, _ *instance.HardwareCharacteristics, err error) {

	name, err := manager.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	defer func() {
		if err != nil {
			callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		}
	}()

	// Set the MachineContainerHostname to match the name returned by virsh list
	instanceConfig.MachineContainerHostname = name

	// Note here that the kvmObjectFactory only returns a valid container
	// object, and doesn't actually construct the underlying kvm container on
	// disk.
	kvmContainer := KvmObjectFactory.New(name)

	// Create the cloud-init.
	directory, err := container.NewDirectory(name)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create container directory")
	}
	logger.Tracef("write cloud-init")
	userDataFilename, err := containerinit.WriteUserData(instanceConfig, networkConfig, directory)
	if err != nil {
		logger.Infof("machine config api %#v", *instanceConfig.APIInfo)
		err = errors.Annotate(err, "failed to write user data")
		logger.Infof(err.Error())
		return nil, nil, err
	}
	// Create the container.
	startParams = ParseConstraintsToStartParams(cons)
	startParams.Arch = arch.HostArch()
	startParams.Series = series
	startParams.Network = networkConfig
	startParams.UserDataFile = userDataFilename

	// If the Simplestream requested is anything but released, update
	// our StartParams to request it.
	if instanceConfig.ImageStream != imagemetadata.ReleasedStream {
		startParams.ImageDownloadUrl = imagemetadata.UbuntuCloudImagesURL + "/" + instanceConfig.ImageStream
	}

	var hardware instance.HardwareCharacteristics
	hardware, err = instance.ParseHardware(
		fmt.Sprintf("arch=%s mem=%vM root-disk=%vG cores=%v",
			startParams.Arch, startParams.Memory, startParams.RootDisk, startParams.CpuCores))
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to parse hardware")
	}

	callback(status.Allocating, "Creating container; it might take some time", nil)
	logger.Tracef("create the container, constraints: %v", cons)
	if err := kvmContainer.Start(startParams); err != nil {
		err = errors.Annotate(err, "kvm container creation failed")
		logger.Infof(err.Error())
		return nil, nil, err
	}
	logger.Tracef("kvm container created")
	return &kvmInstance{kvmContainer, name}, &hardware, nil
}

func (manager *containerManager) IsInitialized() bool {
	requiredBinaries := []string{
		"virsh",
		"uvt-kvm",
	}
	for _, bin := range requiredBinaries {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}
	return true
}

func (manager *containerManager) DestroyContainer(id instance.Id) error {
	name := string(id)
	kvmContainer := KvmObjectFactory.New(name)
	if err := kvmContainer.Stop(); err != nil {
		logger.Errorf("failed to stop kvm container: %v", err)
		return err
	}
	return container.RemoveDirectory(name)
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	containers, err := KvmObjectFactory.List()
	if err != nil {
		logger.Errorf("failed getting all instances: %v", err)
		return
	}
	managerPrefix := manager.namespace.Prefix()
	for _, container := range containers {
		// Filter out those not starting with our name.
		name := container.Name()
		if !strings.HasPrefix(name, managerPrefix) {
			continue
		}
		if container.IsRunning() {
			result = append(result, &kvmInstance{container, name})
		}
	}
	return
}

// ParseConstraintsToStartParams takes a constrants object and returns a bare
// StartParams object that has Memory, Cpu, and Disk populated.  If there are
// no defined values in the constraints for those fields, default values are
// used.  Other constrains cause a warning to be emitted.
func ParseConstraintsToStartParams(cons constraints.Value) StartParams {
	params := StartParams{
		Memory:   DefaultMemory,
		CpuCores: DefaultCpu,
		RootDisk: DefaultDisk,
	}

	if cons.Mem != nil {
		mem := *cons.Mem
		if mem < MinMemory {
			params.Memory = MinMemory
		} else {
			params.Memory = mem
		}
	}
	if cons.CpuCores != nil {
		cores := *cons.CpuCores
		if cores < MinCpu {
			params.CpuCores = MinCpu
		} else {
			params.CpuCores = cores
		}
	}
	if cons.RootDisk != nil {
		size := *cons.RootDisk / 1024
		if size < MinDisk {
			params.RootDisk = MinDisk
		} else {
			params.RootDisk = size
		}
	}
	if cons.Arch != nil {
		logger.Infof("arch constraint of %q being ignored as not supported", *cons.Arch)
	}
	if cons.Container != nil {
		logger.Infof("container constraint of %q being ignored as not supported", *cons.Container)
	}
	if cons.CpuPower != nil {
		logger.Infof("cpu-power constraint of %v being ignored as not supported", *cons.CpuPower)
	}
	if cons.Tags != nil {
		logger.Infof("tags constraint of %q being ignored as not supported", strings.Join(*cons.Tags, ","))
	}

	return params
}
