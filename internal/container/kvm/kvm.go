// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/containerinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
)

var (
	logger = loggo.GetLogger("juju.container.kvm")

	// KVMObjectFactory implements the container factory interface for kvm
	// containers.
	// TODO (stickupkid): This _only_ exists here because we can patch it in
	// tests. This is horrid!
	KVMObjectFactory ContainerFactory = &containerFactory{
		fetcher: simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory()),
	}

	// In order for Juju to be able to create the hardware characteristics of
	// the kvm machines it creates, we need to be explicit in our definition
	// of memory, cores and root-disk.  The defaults here have been
	// extracted from the uvt-kvm executable.

	// DefaultMemory is the default RAM to use in a container.
	DefaultMemory uint64 = 512 // MB
	// DefaultCpu is the default number of CPUs to use in a container.
	DefaultCpu uint64 = 1
	// DefaultDisk is the default root disk size.
	DefaultDisk uint64 = 8 // GB

	// There are some values where it doesn't make sense to go below.

	// MinMemory is the minimum RAM we will launch with.
	MinMemory uint64 = 512 // MB
	// MinCpu is the minimum number of CPUs to launch with.
	MinCpu uint64 = 1
	// MinDisk is the minimum root disk size we will launch with.
	MinDisk uint64 = 2 // GB
)

// Utilized to provide a hard-coded path to kvm-ok
var kvmPath = "/usr/sbin"

// IsKVMSupported calls into the kvm-ok executable from the cpu-checkers package.
// It is a variable to allow us to override behaviour in the tests.
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

	availabilityZone := conf.PopValue(container.ConfigAvailabilityZone)
	if availabilityZone == "" {
		logger.Infof("Availability zone will be empty for this container manager")
	}

	imageMetaDataURL := conf.PopValue(config.ContainerImageMetadataURLKey)
	imageStream := conf.PopValue(config.ContainerImageStreamKey)

	conf.WarnAboutUnused()
	return &containerManager{
		namespace:        namespace,
		logDir:           logDir,
		availabilityZone: availabilityZone,
		imageMetadataURL: imageMetaDataURL,
		imageStream:      imageStream,
	}, nil
}

// containerManager handles all of the business logic at the juju specific
// level. It makes sure that the necessary directories are in place, that the
// user-data is written out in the right place, and that OS images are sourced
// from the correct location.
type containerManager struct {
	namespace        instance.Namespace
	logDir           string
	availabilityZone string
	imageMetadataURL string
	imageStream      string
	imageMutex       sync.Mutex
}

var _ container.Manager = (*containerManager)(nil)

// Namespace implements container.Manager.
func (manager *containerManager) Namespace() instance.Namespace {
	return manager.namespace
}

func (manager *containerManager) CreateContainer(
	_ context.Context,
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	base corebase.Base,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (_ instances.Instance, hc *instance.HardwareCharacteristics, err error) {

	name, err := manager.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	defer func() {
		if err != nil {
			_ = callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		}
	}()

	// Set the MachineContainerHostname to match the name returned by virsh list
	instanceConfig.MachineContainerHostname = name

	// Note here that the kvmObjectFactory only returns a valid container
	// object, and doesn't actually construct the underlying kvm container on
	// disk.
	kvmContainer := KVMObjectFactory.New(name)

	hc = &instance.HardwareCharacteristics{AvailabilityZone: &manager.availabilityZone}

	// Create the cloud-init.
	cloudConfig, err := cloudinit.New(instanceConfig.Base.OS)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	logger.Tracef("write cloud-init")
	userData, err := containerinit.CloudInitUserData(cloudConfig, instanceConfig, networkConfig)
	if err != nil {
		logger.Infof("machine config api %#v", *instanceConfig.APIInfo)
		err = errors.Annotate(err, "failed to write generate data")
		logger.Errorf(err.Error())
		return nil, nil, errors.Trace(err)
	}

	directory, err := container.NewDirectory(name)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create container directory")
	}

	userDataFilename := filepath.Join(directory, "cloud-init")
	if err := os.WriteFile(userDataFilename, userData, 0644); err != nil {
		err = errors.Annotate(err, "failed to write generate data")
		logger.Errorf(err.Error())
		return nil, nil, errors.Trace(err)
	}

	// Create the container.
	startParams := ParseConstraintsToStartParams(cons)
	startParams.Arch = arch.HostArch()
	startParams.Version = base.Channel.Track
	startParams.Network = networkConfig
	startParams.UserDataFile = userDataFilename
	startParams.NetworkConfigData = cloudinit.CloudInitNetworkConfigDisabled
	startParams.StatusCallback = callback
	startParams.Stream = manager.imageStream

	// Check whether a container image metadata URL was configured.
	// Default to Ubuntu cloud images if configured stream is not "released".
	imURL := manager.imageMetadataURL
	if manager.imageMetadataURL == "" && manager.imageStream != imagemetadata.ReleasedStream {
		imURL = imagemetadata.UbuntuCloudImagesURL
		imURL, err = imagemetadata.ImageMetadataURL(imURL, manager.imageStream)
		if err != nil {
			return nil, nil, errors.Annotate(err, "generating image metadata source")
		}
	}
	startParams.ImageDownloadURL = imURL

	var hardware instance.HardwareCharacteristics
	hardware, err = instance.ParseHardware(
		fmt.Sprintf("arch=%s mem=%vM root-disk=%vG cores=%v",
			startParams.Arch, startParams.Memory, startParams.RootDisk, startParams.CpuCores))
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to parse hardware")
	}

	_ = callback(status.Provisioning, "Creating container; it might take some time", nil)
	logger.Tracef("create the container, constraints: %v", cons)

	// Lock around finding an image.
	// The provisioner works concurrently to create containers.
	// If an image needs to be copied from a remote, we don't want many
	// goroutines attempting to do it at once.
	manager.imageMutex.Lock()
	err = kvmContainer.EnsureCachedImage(startParams)
	manager.imageMutex.Unlock()
	if err != nil {
		return nil, nil, errors.Annotate(err, "acquiring container image")
	}

	if err := kvmContainer.Start(startParams); err != nil {
		return nil, nil, errors.Annotate(err, "kvm container creation failed")
	}
	logger.Tracef("kvm container created")
	_ = callback(status.Running, "Container started", nil)
	return &kvmInstance{kvmContainer, name}, &hardware, nil
}

func (manager *containerManager) IsInitialized() bool {
	requiredBinaries := []string{
		"virsh",
		"qemu-utils",
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
	kvmContainer := KVMObjectFactory.New(name)
	if err := kvmContainer.Stop(); err != nil {
		logger.Errorf("failed to stop kvm container: %v", err)
		return err
	}
	return container.RemoveDirectory(name)
}

func (manager *containerManager) ListContainers() (result []instances.Instance, err error) {
	containers, err := KVMObjectFactory.List()
	if err != nil {
		logger.Errorf("failed getting all instances: %v", err)
		return
	}
	managerPrefix := manager.namespace.Prefix()
	for _, c := range containers {
		// Filter out those not starting with our name.
		name := c.Name()
		if !strings.HasPrefix(name, managerPrefix) {
			continue
		}
		if c.IsRunning() {
			result = append(result, &kvmInstance{c, name})
		}
	}
	return
}

// ParseConstraintsToStartParams takes a constraints object and returns a bare
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
