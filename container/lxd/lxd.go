// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools/lxdclient"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

const lxdDefaultProfileName = "default"

// LXD templates README.JUJU content.
const readme = `
Juju Networking
===============

interfaces.tpl and cloud-init-network.tpl have been rewritten by Juju.
Depending on the distro and cloud-init version, either may be used to
create the network configuration in /etc/network/interfaces.d/.

As examples:
 * cloud-init 0.7.5 uses interfaces.tpl (trusty)
 * cloud-init 0.7.7 uses cloud-init-networking.tpl (xenial)
`

// XXX: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.
type containerManager struct {
	modelUUID string
	namespace instance.Namespace
	// A cached client.
	client *lxdclient.Client
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

	client, err := lxdclient.Connect(cfg)
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

	conf.WarnAboutUnused()
	return &containerManager{
		modelUUID: modelUUID,
		namespace: namespace,
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
	callback container.StatusCallback,
) (inst instance.Instance, _ *instance.HardwareCharacteristics, err error) {

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

	err = manager.client.EnsureImageExists(series,
		lxdclient.DefaultImageSources,
		func(progress string) {
			callback(status.Provisioning, progress, nil)
		})
	if err != nil {
		err = errors.Annotatef(err, "failed to ensure LXD image")
		return
	}

	name, err := manager.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Do not pass networkConfig, as we want to inject our own templates instead
	userData, err := containerinit.CloudInitUserData(instanceConfig, nil)
	if err != nil {
		return
	}

	metadata := map[string]string{
		lxdclient.UserdataKey: string(userData),
		// An extra piece of info to let people figure out where this
		// thing came from.
		"user.juju-model": manager.modelUUID,

		// Make sure these come back up on host reboot.
		"boot.autostart": "true",
	}

	nics, err := networkDevices(networkConfig)
	if err != nil {
		return
	}

	// TODO This might be dead code. Do we always get nics > 0?
	profiles := []string{}

	if len(nics) == 0 {
		logger.Infof("instance %q configured with %q profile", name, lxdDefaultProfileName)
		profiles = append(profiles, lxdDefaultProfileName)
	} else {
		logger.Infof("instance %q configured with %v network devices", name, nics)
	}

	// Update the LXD network templates to not rely on DHCP.
	// Do not let the container expect DHCP on eth0, but instead write out
	// ENI before the container starts. Especially on a multi-nic host, it
	// is possible for MAAS to provide DHCP on a different space to that
	// which the container eth0 interface will be bridged.
	eni, err := containerinit.GenerateEtcNetworkInterfaces(networkConfig)
	if err != nil {
		err = errors.Annotatef(err, "failed to generate interfaces.tpl content")
		return
	}

	cinet, err := containerinit.GenerateNetworkConfigV1(networkConfig)
	if err != nil {
		err = errors.Annotatef(err, "failed to generate cloud-init-network.tpl content")
		return
	}

	templates := lxdclient.Templates{
		lxdclient.Template{
			Content: []byte(readme[1:]),
			Name:    "README.JUJU",
		},
		lxdclient.Template{
			Content: []byte(eni),
			Name:    "interfaces.tpl",
		},
		lxdclient.Template{
			Content: []byte(cinet),
			Name:    "cloud-init-network.tpl",
		},
	}

	spec := lxdclient.InstanceSpec{
		Name:      name,
		Image:     manager.client.ImageNameForSeries(series),
		Metadata:  metadata,
		Devices:   nics,
		Profiles:  profiles,
		Templates: templates,
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

func nicDevice(deviceName, parentDevice, hwAddr string, mtu int) (lxdclient.Device, error) {
	device := make(lxdclient.Device)

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

func networkDevices(networkConfig *container.NetworkConfig) (lxdclient.Devices, error) {
	nics := make(lxdclient.Devices)

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
