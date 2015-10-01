// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxd/lxd_client"
	"github.com/juju/juju/instance"
)

var logger = loggo.GetLogger("container.lxd")

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
	client *lxd_client.Client
}

// NewContainerManager builds a new instance of container.Manager and
// returns that new manager.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	namespace := conf.PopValue(container.ConfigName)
	if namespace == "" {
		return nil, errors.Errorf("namespace is required")
	}
	remote := "" // localhost over unix socket
	manager := &containerManager{
		namespace: namespace,
		remote:    remote,
	}

	client, err := manager.connect()
	if err != nil {
		return nil, errors.Trace(err)
	}
	manager.client = client

	return manager, nil
}

func (manager *containerManager) connect() (*lxd_client.Client, error) {
	// TODO: this is going to write the config in the user's home
	// directory, which is (probably) not what we want long term. You can
	// set where the config goes via the LXD API (although it is a bit
	// obtuse), but I'm not sure what path we should use.

	lxdConfig := lxd_client.Config{
		Namespace: manager.namespace,
		Remote:    manager.remote,
	}
	if err := lxdConfig.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	client, err := lxd_client.Connect(lxdConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

// IsInitialized implements container.Manager.
func (manager *containerManager) IsInitialized() bool {
	if manager.client != nil {
		return true
	}

	// NewClient does a roundtrip to the server to make sure it understands
	// the versions, so all we need to do is connect above and we're done.
	client, err := manager.connect()
	if err != nil {
		return false
	}
	manager.client = client

	return true
}

// ListContainers implements container.Manager.
func (manager *containerManager) ListContainers() ([]instance.Instance, error) {
	var result []instance.Instance

	rawInsts, err := manager.client.Instances("")
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, raw := range rawInsts {
		inst := &lxdInstance{
			raw:    &raw,
			client: manager.client,
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
	spec := lxd_client.InstanceSpec{
		Name: name,
	}

	rawInst, err := manager.client.AddInstance(spec)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	inst := &lxdInstance{
		raw:    rawInst,
		client: manager.client,
	}
	var hwc *instance.HardwareCharacteristics
	return inst, hwc, nil
}

// DestroyContainer implements container.Manager.
func (manager *containerManager) DestroyContainer(id instance.Id) error {
	name := string(id)

	if err := manager.client.RemoveInstances("", name); err != nil {
		return errors.Trace(err)
	}

	return nil
}
