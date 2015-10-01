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

// TODO(ericsnow) Move this check to a test suite.
var _ container.Manager = (*containerManager)(nil)

// TODO: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.

// containerManager is an implementation of container.Manager for LXD.
type containerManager struct {
	namespace string
	remote    string
	// A cached client.
	client *lxd.Client
}

// NewContainerManager builds a new instance of container.Manager and
// returns that new manager.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	namespace := conf.PopValue(container.ConfigName)
	if namespace == "" {
		return nil, errors.Errorf("namespace is required")
	}

	remote := "" // localhost over unix socket
	client, err := connect(remote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	manager := &containerManager{
		namespace: namespace,
		remote:    remote,
		client:    client,
	}
	return manager, nil
}

func connect(remote string) (*lxd.Client, error) {
	// TODO: this is going to write the config in the user's home
	// directory, which is (probably) not what we want long term. You can
	// set where the config goes via the LXD API (although it is a bit
	// obtuse), but I'm not sure what path we should use.
	cfg, err := lxd.LoadConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return lxd.NewClient(cfg, remote)
}

// IsInitialized implements container.Manager.
func (manager *containerManager) IsInitialized() bool {
	if manager.client != nil {
		return true
	}

	// NewClient does a roundtrip to the server to make sure it understands
	// the versions, so all we need to do is connect above and we're done.
	client, err := connect(manager.remote)
	if err != nil {
		return false
	}

	manager.client = client
	return true
}

// ListContainers implements container.Manager.
func (manager *containerManager) ListContainers() ([]instance.Instance, error) {
	var result []instance.Instance

	infos, err := manager.client.ListContainers()
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, info := range infos {
		inst := &lxdInstance{
			info.State.Name,
			manager.client,
		}
		result = append(result, inst)
	}

	return result, nil
}

// CreateContainer implements container.Manager.
func (manager *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	name := names.NewMachineTag(instanceConfig.MachineId).String()
	if manager.namespace != "" {
		name = fmt.Sprintf("%s-%s", manager.namespace, name)
	}
	// TODO: FIXME: XXX: don't hardcode ubuntu
	imageAlias := "ubuntu"
	var profiles *[]string
	ephemeral := false
	resp, err := manager.client.Init(name, manager.remote, imageAlias, profiles, ephemeral)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Init is an async operation, since the tar -xvf (or whatever) might
	// take a while; the result is an LXD operation id, which we can just
	// wait on until it is finished.
	if err = manager.client.WaitForSuccess(resp.Operation); err != nil {
		return nil, nil, errors.Trace(err)
	}

	resp, err = manager.client.Action(name, shared.Start, -1, false)
	if err != nil {
		// Try to clean up, but just do it async (i.e. don't
		// WaitForSuccess) since we can't do much if this fails...
		manager.client.Delete(name)
		return nil, nil, errors.Trace(err)
	}

	if err = manager.client.WaitForSuccess(resp.Operation); err != nil {
		return nil, nil, errors.Trace(err)
	}

	inst := &lxdInstance{
		id:     name,
		client: manager.client,
	}
	var hwc *instance.HardwareCharacteristics
	return inst, hwc, nil
}

// DestroyContainer implements container.Manager.
func (manager *containerManager) DestroyContainer(id instance.Id) error {
	resp, err := manager.client.Delete(string(id))
	if err != nil {
		return errors.Trace(err)
	}

	return manager.client.WaitForSuccess(resp.Operation)
}
