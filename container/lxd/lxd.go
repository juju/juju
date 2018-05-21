// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"

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
	"github.com/juju/juju/tools/lxdclient"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

const lxdDefaultProfileName = "default"

// XXX: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.
type containerManager struct {
	modelUUID string
	namespace instance.Namespace
	// A cached client.
	client *lxdclient.Client
	// a host machine's availability zone
	availabilityZone string

	imageMetadataURL string
	imageStream      string
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

func ConnectLocal() (*lxdclient.Client, error) {
	cfg := lxdclient.Config{
		Remote: lxdclient.Local,
	}

	cfg, err := cfg.WithDefaults()
	if err != nil {
		return nil, errors.Trace(err)
	}

	client, err := lxdclient.Connect(cfg, false)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}

// NewContainerManager creates the entity that knows how to create and manage
// LXD containers.
// TODO(jam): This needs to grow support for things like LXC's ImageURLGetter
// functionality.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	modelUUID := conf.PopValue(container.ConfigModelUUID)
	if modelUUID == "" {
		return nil, errors.Errorf("model UUID is required")
	}
	namespace, err := instance.NewNamespace(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	availabilityZone := conf.PopValue(container.ConfigAvailabilityZone)
	if availabilityZone == "" {
		logger.Infof("Availability zone will be empty for this container manager")
	}

	imageMetaDataURL := conf.PopValue(config.ContainerImageMetadataURLKey)
	imageStream := conf.PopValue(config.ContainerImageStreamKey)

	conf.WarnAboutUnused()
	return &containerManager{
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

func (manager *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (inst instance.Instance, hc *instance.HardwareCharacteristics, err error) {

	defer func() {
		if err != nil {
			callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		}
	}()

	if manager.client == nil {
		manager.client, err = ConnectLocal()
		if err != nil {
			err = errors.Annotatef(err, "failed to connect to local LXD")
			return
		}
	}

	// It is only possible to provision LXD containers
	// of the same architecture as the host.
	hostArch := arch.HostArch()

	hc = &instance.HardwareCharacteristics{AvailabilityZone: &manager.availabilityZone}

	imageSources, err := manager.getImageSources()
	if err != nil {
		err = errors.Trace(err)
		return
	}
	imageName, err := manager.client.EnsureImageExists(
		series,
		hostArch,
		imageSources,
		func(progress string) {
			callback(status.Provisioning, progress, nil)
		},
	)
	if err != nil {
		err = errors.Annotatef(err, "failed to ensure LXD image")
		return
	}

	name, err := manager.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Do not pass networkConfig, as we want to directly inject our own ENI
	// rather than using cloud-init.
	userData, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		return
	}

	metadata := map[string]string{
		lxdclient.UserdataKey:      string(userData),
		lxdclient.NetworkconfigKey: containerinit.CloudInitNetworkConfigDisabled,
		// An extra piece of info to let people figure out where this
		// thing came from.
		"user.juju-model": manager.modelUUID,

		// Make sure these come back up on host reboot.
		"boot.autostart": "true",
	}

	nics, unknown, err := networkDevicesFromConfig(networkConfig)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var profiles []string
	if len(nics) == 0 {
		logger.Infof("configuring container %q with %q profile", name, lxdDefaultProfileName)
		profiles = []string{lxdDefaultProfileName}
	} else {
		logger.Infof("configuring container %q with network devices: %v", name, nics)

		// If the default LXD bridge was supplied in network config,
		// but without a CIDR, attempt to ensure it is configured for IPv4.
		// If there are others with incomplete info, or if the network API is
		// not available for us to modify the network config, then log a
		// warning.
		if len(unknown) > 0 {
			logWarning := true
			if len(unknown) == 1 && unknown[0] == network.DefaultLXDBridge {
				mod, err := manager.ensureIPv4(network.DefaultLXDBridge)
				if err != nil && !errors.IsNotSupported(err) {
					return nil, nil, errors.Annotate(err, "ensuring default bridge IPv4 config")
				}
				if mod {
					logWarning = false
					logger.Infof(`added "auto" IPv4 configuration to default LXD bridge`)
				}
			}
			if logWarning {
				logger.Warningf("no CIDR was detected for the following networks: %v", unknown)
			}
		}
	}

	spec := lxdclient.InstanceSpec{
		Name:     name,
		Image:    imageName,
		Metadata: metadata,
		Devices:  nics,
		Profiles: profiles,
	}

	logger.Infof("starting instance %q (image %q)...", spec.Name, spec.Image)
	callback(status.Provisioning, "Starting container", nil)
	_, err = manager.client.AddInstance(spec)
	if err != nil {
		return
	}

	callback(status.Running, "Container started", nil)
	inst = &lxdInstance{name, manager.client}
	return
}

// getImageSources returns a list of LXD remote image sources based on the
// configuration that was passed into the container manager.
func (manager *containerManager) getImageSources() ([]lxdclient.Remote, error) {
	imURL := manager.imageMetadataURL

	// Unless the configuration explicitly requests the daily stream,
	// an empty image metadata URL results in a search of the default sources.
	if imURL == "" && manager.imageStream != "daily" {
		logger.Debugf("checking default image metadata sources")
		return []lxdclient.Remote{lxdclient.CloudImagesRemote, lxdclient.CloudImagesDailyRemote}, nil
	}
	// Otherwise only check the daily stream.
	if imURL == "" {
		return []lxdclient.Remote{lxdclient.CloudImagesDailyRemote}, nil
	}

	imURL, err := imagemetadata.ImageMetadataURL(imURL, manager.imageStream)
	if err != nil {
		return nil, errors.Annotatef(err, "generating image metadata source")
	}
	// LXD requires HTTPS.
	imURL = strings.Replace(imURL, "http:", "https:", 1)
	remote := lxdclient.Remote{
		Name:          strings.Replace(imURL, "https://", "", 1),
		Host:          imURL,
		Protocol:      lxdclient.SimplestreamsProtocol,
		Cert:          nil,
		ServerPEMCert: "",
	}

	// If the daily stream was configured with custom image metadata URL,
	// only use the Ubuntu daily as a fallback.
	if manager.imageStream == "daily" {
		return []lxdclient.Remote{remote, lxdclient.CloudImagesDailyRemote}, nil
	}
	return []lxdclient.Remote{remote, lxdclient.CloudImagesRemote, lxdclient.CloudImagesDailyRemote}, nil
}

// ensureIPv4 retrieves the network for the input name and checks its IPv4
// configuration. If none is detected, it is set to "auto".
// The boolean return indicates if modification was necessary.
func (manager *containerManager) ensureIPv4(netName string) (bool, error) {
	var modified bool

	net, err := manager.client.NetworkGet(netName)
	if err != nil {
		return false, err
	}

	cfg, ok := net.Config["ipv4.address"]
	if !ok || cfg == "none" {
		if net.Config == nil {
			net.Config = make(map[string]string, 2)
		}
		net.Config["ipv4.address"] = "auto"
		net.Config["ipv4.nat"] = "true"

		if err := manager.client.NetworkPut(netName, net.Writable()); err != nil {
			return false, err
		}
		modified = true
	}

	return modified, nil
}

func (manager *containerManager) DestroyContainer(id instance.Id) error {
	if manager.client == nil {
		var err error
		manager.client, err = ConnectLocal()
		if err != nil {
			return err
		}
	}
	return errors.Trace(manager.client.RemoveInstances(manager.namespace.Prefix(), string(id)))
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	result = []instance.Instance{}
	if manager.client == nil {
		manager.client, err = ConnectLocal()
		if err != nil {
			return
		}
	}

	lxdInstances, err := manager.client.Instances(manager.namespace.Prefix())
	if err != nil {
		return
	}

	for _, i := range lxdInstances {
		result = append(result, &lxdInstance{i.Name, manager.client})
	}

	return
}

func (manager *containerManager) IsInitialized() bool {
	if manager.client != nil {
		return true
	}

	// NewClient does a roundtrip to the server to make sure it understands
	// the versions, so all we need to do is connect above and we're done.
	var err error
	manager.client, err = ConnectLocal()
	return err == nil
}

// HasLXDSupport returns false when this juju binary was not built with LXD
// support (i.e. it was built on a golang version < 1.2
func HasLXDSupport() bool {
	return true
}

// networkDevicesFromConfig uses the input container network configuration to
// create a map of network device configuration in the LXD format.
// Names for any networks without a known CIDR are returned in a slice.
func networkDevicesFromConfig(networkConfig *container.NetworkConfig) (
	lxdclient.Devices, []string, error,
) {
	nics := make(lxdclient.Devices)
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
func newNICDevice(deviceName, parentDevice, hwAddr string, mtu int) lxdclient.Device {
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
