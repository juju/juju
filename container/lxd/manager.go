// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuarch "github.com/juju/utils/arch"

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
	return errors.Trace(m.server.RemoveContainer(string(id)))
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
	spec, err := m.getContainerSpec(instanceConfig, cons, series, networkConfig, storageConfig, callback)
	logger.Infof("starting container %q (image %q)...", spec.Name, spec.Image.Image.Filename)

	callback(status.Provisioning, "Creating container", nil)
	c, err := m.server.CreateContainerFromSpec(spec)
	if err != nil {
		callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}

	callback(status.Running, "Container started", nil)
	return &lxdInstance{c.Name, m.server.ContainerServer},
		&instance.HardwareCharacteristics{AvailabilityZone: &m.availabilityZone}, nil
}

// ListContainers implements container.Manager.
func (m *containerManager) ListContainers() ([]instance.Instance, error) {
	containers, err := m.server.FilterContainers(m.namespace.Prefix())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []instance.Instance
	for _, i := range containers {
		result = append(result, &lxdInstance{i.Name, m.server.ContainerServer})
	}
	return result, nil
}

// IsInitialized implements container.Manager.
func (m *containerManager) IsInitialized() bool {
	return m.server != nil
}

// getContainerSpec generates a spec for creating a new container.
// It sources an image based on the input series, and transforms the input
// config objects into LXD configuration, including cloud init user data.
func (m *containerManager) getContainerSpec(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (ContainerSpec, error) {
	imageSources, err := m.getImageSources()
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	found, err := m.server.FindImage(series, jujuarch.HostArch(), imageSources, true, callback)
	if err != nil {
		return ContainerSpec{}, errors.Annotatef(err, "failed to ensure LXD image")
	}

	name, err := m.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	// CloudInitUserData creates our own ENI/netplan.
	// We need to disable cloud-init networking to make it work.
	userData, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
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
		return ContainerSpec{}, errors.Trace(err)
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
					return ContainerSpec{}, errors.Annotate(err, "ensuring default bridge IPv4 config")
				}
				if mod {
					logger.Infof(`added "auto" IPv4 configuration to default LXD bridge`)
				}
			} else {
				logger.Warningf("no CIDR was detected for the following networks: %v", unknown)
			}
		}
	}

	return ContainerSpec{
		Name:     name,
		Profiles: profiles,
		Image:    found,
		Config:   cfg,
		Devices:  nics,
	}, nil
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
