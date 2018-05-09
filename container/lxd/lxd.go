// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuarch "github.com/juju/utils/arch"
	"github.com/lxc/lxd/client"
	lxdshared "github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

const lxdDefaultProfileName = "default"
const UserDataKey = "user.user-data"
const NetworkConfigKey = "user.network-config"
const JujuModelKey = "user.juju-model"
const AutoStartKey = "boot.autostart"

// XXX: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.
type containerManager struct {
	server      lxd.ContainerServer
	imageServer *JujuImageServer

	modelUUID        string
	namespace        instance.Namespace
	availabilityZone string

	imageMetadataURL string
	imageStream      string
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

var generateCertificate = func() ([]byte, []byte, error) { return lxdshared.GenerateMemCert(true) }

// NewContainerManager creates the entity that knows how to create and manage
// LXD containers.
// TODO(jam): This needs to grow support for things like LXC's ImageURLGetter
// functionality.
func NewContainerManager(cfg container.ManagerConfig, server lxd.ContainerServer) (container.Manager, error) {
	modelUUID := cfg.PopValue(container.ConfigModelUUID)
	if modelUUID == "" {
		return nil, errors.Errorf("model UUID is required")
	}
	namespace, err := instance.NewNamespace(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	availabilityZone := cfg.PopValue(container.ConfigAvailabilityZone)
	if availabilityZone == "" {
		logger.Infof("Availability zone will be empty for this container manager")
	}

	imageMetaDataURL := cfg.PopValue(config.ContainerImageMetadataURLKey)
	imageStream := cfg.PopValue(config.ContainerImageStreamKey)

	cfg.WarnAboutUnused()
	return &containerManager{
		server:           server,
		imageServer:      &JujuImageServer{server},
		modelUUID:        modelUUID,
		namespace:        namespace,
		availabilityZone: availabilityZone,
		imageMetadataURL: imageMetaDataURL,
		imageStream:      imageStream,
	}, nil
}

// Namespace implements container.Manager.
func (manager *containerManager) Namespace() instance.Namespace {
	return manager.namespace
}

// DestroyContainer implements container.Manager.
func (manager *containerManager) DestroyContainer(id instance.Id) error {
	if err := manager.stopInstance(string(id)); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(manager.removeInstance(string(id)))
}

// CreateContainer implements container.Manager.
func (manager *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	callback(status.Provisioning, "Creating container", nil)
	name, err := manager.createInstance(instanceConfig, cons, series, networkConfig, storageConfig, callback)
	if err != nil {
		callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}

	callback(status.Provisioning, "Container created, starting", nil)
	err = manager.startInstance(name)
	if err != nil {
		if err := manager.removeInstance(name); err != nil {
			logger.Errorf("Cannot remove failed instance: %s", err)
		}
		callback(status.ProvisioningError, fmt.Sprintf("Starting container: %v", err), nil)
		return nil, nil, err
	}

	callback(status.Running, "Container started", nil)
	return &lxdInstance{name, manager.server},
		&instance.HardwareCharacteristics{AvailabilityZone: &manager.availabilityZone}, nil
}

// ListContainers implements container.Manager.
func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	result = []instance.Instance{}
	lxdInstances, err := manager.server.GetContainers()
	if err != nil {
		return
	}

	for _, i := range lxdInstances {
		if strings.HasPrefix(i.Name, manager.namespace.Prefix()) {
			result = append(result, &lxdInstance{i.Name, manager.server})
		}
	}

	return result, nil
}

// IsInitialized implements container.Manager.
func (manager *containerManager) IsInitialized() bool {
	return manager.server != nil
}

// startInstance starts previously created instance.
func (manager *containerManager) startInstance(name string) error {
	req := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}
	op, err := manager.server.UpdateContainerState(name, req, "")
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

// stopInstance stops instance if it's not stopped.
func (manager *containerManager) stopInstance(name string) error {
	state, etag, err := manager.server.GetContainerState(name)
	if err != nil {
		return err
	}

	if state.StatusCode == api.Stopped {
		return nil
	}

	req := api.ContainerStatePut{
		Action:  "stop",
		Timeout: -1,
	}
	op, err := manager.server.UpdateContainerState(name, req, etag)
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

// createInstance creates a stopped instance from given config. It finds the proper image, either
// locally or remotely, and then creates a container using it.
func (manager *containerManager) createInstance(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (string, error) {
	var err error

	imageSources, err := manager.getImageSources()
	if err != nil {
		return "", errors.Trace(err)
	}

	found, err := manager.imageServer.FindImage(series, jujuarch.HostArch(), imageSources, true, callback)
	if err != nil {
		return "", errors.Annotatef(err, "failed to ensure LXD image")
	}

	name, err := manager.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return "", errors.Trace(err)
	}

	// CloudInitUserData creates our own ENI/netplan, we need to disable cloud-init networking
	// to make it work.
	userData, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		return "", errors.Trace(err)
	}

	cfg := map[string]string{
		UserDataKey:      string(userData),
		NetworkConfigKey: cloudinit.CloudInitNetworkConfigDisabled,
		// An extra piece of info to let people figure out where this
		// thing came from.
		JujuModelKey: manager.modelUUID,
		// Make sure these come back up on host reboot.
		AutoStartKey: "true",
	}

	nics, err := networkDevices(networkConfig)
	if err != nil {
		return "", errors.Trace(err)
	}

	var profiles []string
	if len(nics) == 0 {
		logger.Infof("instance %q configured with %q profile", name, lxdDefaultProfileName)
		profiles = []string{lxdDefaultProfileName}
	} else {
		logger.Infof("instance %q configured with %v network devices", name, nics)
	}

	logger.Infof("starting instance %q (image %q)...", name, found.Image.Fingerprint)
	spec := api.ContainersPost{
		Name: name,
		ContainerPut: api.ContainerPut{
			Profiles: profiles,
			Devices:  nics,
			Config:   cfg,
		},
	}

	callback(status.Provisioning, "Creating container", nil)
	op, err := manager.server.CreateContainerFromImage(found.LXDServer, *found.Image, spec)
	if err != nil {
		logger.Errorf("CreateContainer failed with %s", err)
		return "", errors.Trace(err)
	}

	if err := op.Wait(); err != nil {
		return "", errors.Trace(err)
	}
	opInfo, err := op.GetTarget()
	if err != nil {
		return "", errors.Trace(err)
	}
	if opInfo.StatusCode != api.Success {
		return "", fmt.Errorf("container creation failed: %s", opInfo.Err)
	}
	return name, nil
}

// getImageSources returns a list of LXD remote image sources based on the
// configuration that was passed into the container manager.
func (manager *containerManager) getImageSources() ([]RemoteServer, error) {
	imURL := manager.imageMetadataURL

	// Unless the configuration explicitly requests the daily stream,
	// an empty image metadata URL results in a search of the default sources.
	if imURL == "" && manager.imageStream != "daily" {
		logger.Debugf("checking default image metadata sources")
		return []RemoteServer{CloudImagesRemote, CloudImagesDailyRemote}, nil
	}
	// Otherwise only check the daily stream.
	if imURL == "" {
		return []RemoteServer{CloudImagesDailyRemote}, nil
	}

	imURL, err := imagemetadata.ImageMetadataURL(imURL, manager.imageStream)
	if err != nil {
		return nil, errors.Annotatef(err, "generating image metadata source")
	}
	// LXD requires HTTPS.
	imURL = strings.Replace(imURL, "http:", "https:", 1)
	remote := RemoteServer{
		Name:     strings.Replace(imURL, "https://", "", 1),
		Host:     imURL,
		Protocol: SimpleStreamsProtocol,
	}

	// If the daily stream was configured with custom image metadata URL,
	// only use the Ubuntu daily as a fallback.
	if manager.imageStream == "daily" {
		return []RemoteServer{remote, CloudImagesDailyRemote}, nil
	}
	return []RemoteServer{remote, CloudImagesRemote, CloudImagesDailyRemote}, nil
}

func (manager *containerManager) removeInstance(name string) error {
	op, err := manager.server.DeleteContainer(name)
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

func nicDevice(deviceName, parentDevice, hwAddr string, mtu int) (map[string]string, error) {
	device := make(map[string]string)

	device["type"] = "nic"
	device["nictype"] = "bridged"

	if deviceName == "" {
		return nil, errors.Errorf("invalid device name")
	}
	device["name"] = deviceName

	if parentDevice == "" {
		return nil, errors.Errorf("invalid parent device name")
	}
	device["parent"] = parentDevice

	if hwAddr != "" {
		device["hwaddr"] = hwAddr
	}

	if mtu > 0 {
		device["mtu"] = fmt.Sprintf("%v", mtu)
	}

	return device, nil
}

func networkDevices(networkConfig *container.NetworkConfig) (map[string]map[string]string, error) {
	nics := make(map[string]map[string]string)

	if len(networkConfig.Interfaces) > 0 {
		for _, v := range networkConfig.Interfaces {
			if v.InterfaceType == network.LoopbackInterface {
				continue
			}
			if v.InterfaceType != network.EthernetInterface {
				return nil, errors.Errorf("interface type %q not supported", v.InterfaceType)
			}
			parentDevice := v.ParentInterfaceName
			device, err := nicDevice(v.InterfaceName, parentDevice, v.MACAddress, v.MTU)
			if err != nil {
				return nil, errors.Trace(err)
			}
			nics[v.InterfaceName] = device
		}
	} else if networkConfig.Device != "" {
		device, err := nicDevice("eth0", networkConfig.Device, "", networkConfig.MTU)
		if err != nil {
			return nil, errors.Trace(err)
		}
		nics["eth0"] = device
	}

	return nics, nil
}
