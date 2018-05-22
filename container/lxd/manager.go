// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuarch "github.com/juju/utils/arch"
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

type containerManager struct {
	server *Server

	modelUUID        string
	namespace        instance.Namespace
	availabilityZone string

	imageMetadataURL string
	imageStream      string
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

// NewContainerManager creates the entity that knows how to create and manage
// LXD containers.
// TODO(jam): This needs to grow support for things like LXC's ImageURLGetter
// functionality.
func NewContainerManager(cfg container.ManagerConfig, svr *Server) (container.Manager, error) {
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
		server:           svr,
		modelUUID:        modelUUID,
		namespace:        namespace,
		availabilityZone: availabilityZone,
		imageMetadataURL: imageMetaDataURL,
		imageStream:      imageStream,
	}, nil
}

// Namespace implements container.Manager.
func (m *containerManager) Namespace() instance.Namespace {
	return m.namespace
}

// DestroyContainer implements container.Manager.
func (m *containerManager) DestroyContainer(id instance.Id) error {
	if err := m.stopContainer(string(id)); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(m.deleteContainer(string(id)))
}

// CreateContainer implements container.Manager.
func (m *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	callback(status.Provisioning, "Creating container", nil)
	name, err := m.createContainer(instanceConfig, cons, series, networkConfig, storageConfig, callback)
	if err != nil {
		callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}

	callback(status.Provisioning, "Container created, starting", nil)
	err = m.startContainer(name)
	if err != nil {
		if err := m.deleteContainer(name); err != nil {
			logger.Errorf("Cannot remove failed instance: %s", err)
		}
		callback(status.ProvisioningError, fmt.Sprintf("Starting container: %v", err), nil)
		return nil, nil, err
	}

	callback(status.Running, "Container started", nil)
	return &lxdInstance{name, m.server.ContainerServer},
		&instance.HardwareCharacteristics{AvailabilityZone: &m.availabilityZone}, nil
}

// ListContainers implements container.Manager.
func (m *containerManager) ListContainers() (result []instance.Instance, err error) {
	result = []instance.Instance{}
	lxdInstances, err := m.server.GetContainers()
	if err != nil {
		return
	}

	for _, i := range lxdInstances {
		if strings.HasPrefix(i.Name, m.namespace.Prefix()) {
			result = append(result, &lxdInstance{i.Name, m.server.ContainerServer})
		}
	}

	return result, nil
}

// IsInitialized implements container.Manager.
func (m *containerManager) IsInitialized() bool {
	return m.server != nil
}

// startContainer starts previously created container.
func (m *containerManager) startContainer(name string) error {
	req := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}
	op, err := m.server.UpdateContainerState(name, req, "")
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

// stopContainer stops a container if it is not stopped.
func (m *containerManager) stopContainer(name string) error {
	state, etag, err := m.server.GetContainerState(name)
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
	op, err := m.server.UpdateContainerState(name, req, etag)
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

// createContainer creates a stopped container from given config.
// It finds the proper image, either locally or remotely,
// and then creates a container using it.
func (m *containerManager) createContainer(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (string, error) {
	var err error

	imageSources, err := m.getImageSources()
	if err != nil {
		return "", errors.Trace(err)
	}

	found, err := m.server.FindImage(series, jujuarch.HostArch(), imageSources, true, callback)
	if err != nil {
		return "", errors.Annotatef(err, "failed to ensure LXD image")
	}

	name, err := m.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return "", errors.Trace(err)
	}

	// CloudInitUserData creates our own ENI/netplan.
	// We need to disable cloud-init networking to make it work.
	userData, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		return "", errors.Trace(err)
	}

	cfg := map[string]string{
		UserDataKey:      string(userData),
		NetworkConfigKey: cloudinit.CloudInitNetworkConfigDisabled,
		AutoStartKey:     "true",
		// Extra info to indicate the origin of this container.
		JujuModelKey: m.modelUUID,
	}

	nics, unknown, err := networkDevicesFromConfig(networkConfig)
	if err != nil {
		return "", errors.Trace(err)
	}

	var profiles []string
	if len(nics) == 0 {
		logger.Infof("configuring container %q with %q profile", name, lxdDefaultProfileName)
		profiles = []string{lxdDefaultProfileName}
	} else {
		logger.Infof("configuring container %q with network devices: %v", name, nics)

		// If the default LXD bridge was supplied in network config,
		// but without a CIDR, attempt to ensure it is configured for IPv4.
		// If there are others with incomplete info, log a warning.
		if len(unknown) > 0 {
			if len(unknown) == 1 && unknown[0] == network.DefaultLXDBridge && m.server.networkAPISupport {
				mod, err := m.server.EnsureIPv4(network.DefaultLXDBridge)
				if err != nil {
					return "", errors.Annotate(err, "ensuring default bridge IPv4 config")
				}
				if mod {
					logger.Infof(`added "auto" IPv4 configuration to default LXD bridge`)
				}
			} else {
				logger.Warningf("no CIDR was detected for the following networks: %v", unknown)
			}
		}
	}

	logger.Infof("starting container %q (image %q)...", name, found.Image.Fingerprint)
	spec := api.ContainersPost{
		Name: name,
		ContainerPut: api.ContainerPut{
			Profiles: profiles,
			Devices:  nics,
			Config:   cfg,
		},
	}

	callback(status.Provisioning, "Creating container", nil)
	op, err := m.server.CreateContainerFromImage(found.LXDServer, *found.Image, spec)
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
func (m *containerManager) getImageSources() ([]RemoteServer, error) {
	imURL := m.imageMetadataURL

	// Unless the configuration explicitly requests the daily stream,
	// an empty image metadata URL results in a search of the default sources.
	if imURL == "" && m.imageStream != "daily" {
		logger.Debugf("checking default image metadata sources")
		return []RemoteServer{CloudImagesRemote, CloudImagesDailyRemote}, nil
	}
	// Otherwise only check the daily stream.
	if imURL == "" {
		return []RemoteServer{CloudImagesDailyRemote}, nil
	}

	imURL, err := imagemetadata.ImageMetadataURL(imURL, m.imageStream)
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
	if m.imageStream == "daily" {
		return []RemoteServer{remote, CloudImagesDailyRemote}, nil
	}
	return []RemoteServer{remote, CloudImagesRemote, CloudImagesDailyRemote}, nil
}

func (m *containerManager) deleteContainer(name string) error {
	op, err := m.server.DeleteContainer(name)
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

// networkDevicesFromConfig uses the input container network configuration to
// create a map of network device configuration in the LXD format.
// Names for any networks without a known CIDR are returned in a slice.
func networkDevicesFromConfig(networkConfig *container.NetworkConfig) (
	map[string]map[string]string, []string, error,
) {
	nics := make(map[string]map[string]string, len(networkConfig.Interfaces))
	var unknownNetworks []string

	if len(networkConfig.Interfaces) > 0 {
		for _, v := range networkConfig.Interfaces {
			if v.InterfaceType == network.LoopbackInterface {
				continue
			}
			if v.InterfaceType != network.EthernetInterface {
				return nil, nil, errors.Errorf("interface type %q not supported", v.InterfaceType)
			}
			if v.ParentInterfaceName == "" {
				return nil, nil, errors.Errorf("parent interface name is empty")
			}
			if v.CIDR == "" {
				unknownNetworks = append(unknownNetworks, v.ParentInterfaceName)
			}
			nics[v.InterfaceName] = newNICDevice(v.InterfaceName, v.ParentInterfaceName, v.MACAddress, v.MTU)
		}
	} else if networkConfig.Device != "" {
		unknownNetworks = []string{networkConfig.Device}
		nics["eth0"] = newNICDevice("eth0", networkConfig.Device, "", networkConfig.MTU)
	}

	return nics, unknownNetworks, nil
}

// newNICDevice creates and returns a LXD-compatible config for a network
// device, from the input arguments.
func newNICDevice(deviceName, parentDevice, hwAddr string, mtu int) map[string]string {
	device := map[string]string{
		"type":    "nic",
		"nictype": "bridged",
		"name":    deviceName,
		"parent":  parentDevice,
	}

	if hwAddr != "" {
		device["hwaddr"] = hwAddr
	}

	if mtu > 0 {
		device["mtu"] = fmt.Sprintf("%v", mtu)
	}

	return device
}
