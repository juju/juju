// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/lxc/lxd"

	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools/lxdclient"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

// XXX: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.
type containerManager struct {
	name string
	// A cached client.
	client *lxdclient.Client
	// Custom network profile
	networkProfile string
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

func ConnectLocal(namespace string) (*lxdclient.Client, error) {
	cfg := lxdclient.Config{
		Namespace: namespace,
		Remote:    lxdclient.Local,
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
	name := conf.PopValue(container.ConfigName)
	if name == "" {
		return nil, errors.Errorf("name is required")
	}

	conf.WarnAboutUnused()
	return &containerManager{name: name}, nil
}

func (manager *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback container.StatusCallback,
) (inst instance.Instance, _ *instance.HardwareCharacteristics, err error) {

	defer func() {
		if err != nil {
			manager.deleteNetworkProfile()
			callback(status.StatusProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		}
	}()

	if manager.client == nil {
		manager.client, err = ConnectLocal(manager.name)
		if err != nil {
			err = errors.Annotatef(err, "failed to connect to local LXD")
			return
		}
	}

	err = manager.client.EnsureImageExists(series,
		lxdclient.DefaultImageSources,
		func(progress string) {
			callback(status.StatusProvisioning, progress, nil)
		})
	if err != nil {
		err = errors.Annotatef(err, "failed to ensure LXD image")
		return
	}

	name := names.NewMachineTag(instanceConfig.MachineId).String()
	if manager.name != "" {
		name = fmt.Sprintf("%s-%s", manager.name, name)
	}

	userData, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		return
	}

	metadata := map[string]string{
		lxdclient.UserdataKey: string(userData),
		// An extra piece of info to let people figure out where this
		// thing came from.
		"user.juju-environment": manager.name,

		// Make sure these come back up on host reboot.
		"boot.autostart": "true",
	}

	networkProfile := fmt.Sprintf("%s-network", name)

	if err = createNetworkProfile(manager.client, networkProfile); err != nil {
		return
	}

	if len(networkConfig.Interfaces) > 0 || networkConfig.Device != "" {
		manager.networkProfile = networkProfile
		if len(networkConfig.Interfaces) > 0 {
			err = networkProfileAddInterfaces(manager.client, networkProfile, networkConfig)
		} else {
			err = networkProfileAddSingleInterface(manager.client, networkProfile, networkConfig)
		}
		if err != nil {
			return
		}
	} else {
		networkProfile = "default"
	}

	spec := lxdclient.InstanceSpec{
		Name:     name,
		Image:    manager.client.ImageNameForSeries(series),
		Metadata: metadata,
		Profiles: []string{
			networkProfile,
		},
	}

	logger.Infof("starting instance %q (image %q)...", spec.Name, spec.Image)
	callback(status.StatusProvisioning, "Starting container", nil)
	_, err = manager.client.AddInstance(spec)
	if err != nil {
		manager.client.ProfileDelete(networkProfile)
		return
	}

	callback(status.StatusRunning, "Container started", nil)
	inst = &lxdInstance{name, manager.client}
	return
}

func (manager *containerManager) DestroyContainer(id instance.Id) error {
	if manager.client == nil {
		var err error
		manager.client, err = ConnectLocal(manager.name)
		if err != nil {
			return err
		}
	}
	manager.deleteNetworkProfile()
	return errors.Trace(manager.client.RemoveInstances(manager.name, string(id)))
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	result = []instance.Instance{}
	if manager.client == nil {
		manager.client, err = ConnectLocal(manager.name)
		if err != nil {
			return
		}
	}

	lxdInstances, err := manager.client.Instances(manager.name)
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
	manager.client, err = ConnectLocal(manager.name)
	return err == nil
}

// HasLXDSupport returns false when this juju binary was not built with LXD
// support (i.e. it was built on a golang version < 1.2
func HasLXDSupport() bool {
	return true
}

func nicProperties(parentDevice, deviceName, hwAddr string, mtu int) ([]string, error) {
	var props = []string{"nictype=bridged"}

	if parentDevice == "" {
		return nil, errors.Errorf("invalid parent device")
	} else {
		props = append(props, fmt.Sprintf("parent=%v", parentDevice))
	}

	if deviceName == "" {
		return nil, errors.Errorf("invalid device name")
	} else {
		props = append(props, fmt.Sprintf("name=%v", deviceName))
	}

	if hwAddr != "" {
		props = append(props, fmt.Sprintf("hwaddr=%v", hwAddr))
	}

	if mtu > 0 {
		props = append(props, fmt.Sprintf("mtu=%v", mtu))
	}

	return props, nil
}

func addNetworkDeviceToProfile(client *lxdclient.Client, profile, parentDevice, deviceName, hwAddr string, mtu int) (*lxd.Response, error) {
	props, err := nicProperties(parentDevice, deviceName, hwAddr, mtu)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("adding nic device %q with properties %+v to profile %q", deviceName, props, profile)
	return client.ProfileDeviceAdd(profile, deviceName, "nic", props)
}

func networkProfileAddSingleInterface(client *lxdclient.Client, profile string, networkConfig *container.NetworkConfig) error {
	_, err := addNetworkDeviceToProfile(client, profile, networkConfig.Device, "eth0", "", networkConfig.MTU)
	return errors.Trace(err)
}

func networkProfileAddInterfaces(client *lxdclient.Client, profile string, networkConfig *container.NetworkConfig) error {
	for _, v := range networkConfig.Interfaces {
		if v.InterfaceType == network.LoopbackInterface {
			continue
		}

		if v.InterfaceType != network.EthernetInterface {
			return errors.Errorf("interface type %q not supported", v.InterfaceType)
		}

		_, err := addNetworkDeviceToProfile(client, profile, v.ParentInterfaceName, v.InterfaceName, v.MACAddress, v.MTU)

		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func createNetworkProfile(client *lxdclient.Client, profile string) error {
	found, err := client.HasProfile(profile)

	if err != nil {
		return errors.Trace(err)
	}

	if found {
		logger.Infof("deleting existing container profile %q", profile)
		if err := client.ProfileDelete(profile); err != nil {
			return errors.Trace(err)
		}
	}

	err = client.CreateProfile(profile, nil)

	if err == nil {
		logger.Infof("created new network container profile %q", profile)
	}

	return errors.Trace(err)
}

func (manager *containerManager) deleteNetworkProfile() {
	if manager.client != nil && manager.networkProfile != "" {
		logger.Infof("deleting container network profile %q", manager.networkProfile)
		if err := manager.client.ProfileDelete(manager.networkProfile); err != nil {
			logger.Warningf("discarding profile delete error: %v", err)
		}
		manager.networkProfile = ""
	}
}
