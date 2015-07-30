// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
)

// XXX: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.
type containerManager struct {
	name string
	// A cached client.
	client *lxd.Client
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

func (manager *containerManager) connect() (*lxd.Client, error) {
	// TODO: this is going to write the config in the user's home
	// directory, which is (probably) not what we want long term. You can
	// set where the config goes via the LXD API (although it is a bit
	// obtuse), but I'm not sure what path we should use.
	cfg, err := lxd.LoadConfig()
	if err != nil {
		return nil, err
	}

	// "" == localhost over unix socket
	return lxd.NewClient(cfg, "")
}

func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	name := conf.PopValue(container.ConfigName)
	if name == "" {
		return nil, errors.Errorf("name is required")
	}

	return &containerManager{name: name}, nil
}

func (manager *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
) (inst instance.Instance, _ *instance.HardwareCharacteristics, err error) {
	if manager.client == nil {
		manager.client, err = manager.connect()
		if err != nil {
			return
		}
	}

	name := names.NewMachineTag(instanceConfig.MachineId).String()
	if manager.name != "" {
		name = fmt.Sprintf("%s-%s", manager.name, name)
	}

	// TODO: FIXME: XXX: don't hardcode ubuntu
	init, err := manager.client.Init(name, "", "ubuntu", nil, false)
	if err != nil {
		return
	}

	// Init is an async operation, since the tar -xvf (or whatever) might
	// take a while; the result is an LXD operation id, which we can just
	// wait on until it is finished.
	if err = manager.client.WaitForSuccess(init.Operation); err != nil {
		return
	}

	start, err := manager.client.Action(name, shared.Start, -1, false)
	if err != nil {
		// Try to clean up, but just do it async (i.e. don't
		// WaitForSuccess) since we can't do much if this fails...
		manager.client.Delete(name)
		return
	}

	if err = manager.client.WaitForSuccess(start.Operation); err != nil {
		return
	}

	inst = &lxdInstance{name, manager.client}
	return
}

func (manager *containerManager) DestroyContainer(id instance.Id) error {
	if manager.client == nil {
		var err error
		manager.client, err = manager.connect()
		if err != nil {
			return err
		}
	}

	resp, err := manager.client.Delete(string(id))
	if err != nil {
		return err
	}

	return manager.client.WaitForSuccess(resp.Operation)
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	result = []instance.Instance{}
	if manager.client == nil {
		manager.client, err = manager.connect()
		if err != nil {
			return nil, err
		}
	}

	containers, err := manager.client.ListContainers()
	if err != nil {
		return nil, err
	}

	for _, container := range containers {
		result = append(result, &lxdInstance{container.State.Name, manager.client})
	}

	return result, nil
}

func (manager *containerManager) IsInitialized() bool {
	if manager.client != nil {
		return true
	}

	// NewClient does a roundtrip to the server to make sure it understands
	// the versions, so all we need to do is connect above and we're done.
	var err error
	manager.client, err = manager.connect()
	return err == nil
}
