// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools/lxdclient"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

const (
	DefaultLxdBridge = "lxcbr0"
)

// XXX: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.
type containerManager struct {
	name string
	// A cached client.
	client *lxdclient.Client
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

func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	name := conf.PopValue(container.ConfigName)
	if name == "" {
		return nil, errors.Errorf("name is required")
	}

	conf.WarnAboutUnused()
	return &containerManager{name: name}, nil
}

type progressForwarderWithDiscarding struct {
	callback     container.StatusCallback
	buffer       chan string
	stop         chan struct{}
	discardCount int
}

// NewProgressForwarder creates an object that can forward string messages into
// a StatusCallback function. However if it receives messages faster than
// callback can handle, it will discard them.
func NewProgressForwarder(callback container.StatusCallback) *progressForwarderWithDiscarding {
	forwarder := &progressForwarderWithDiscarding{
		callback:     callback,
		buffer:       make(chan string, 1),
		stop:         make(chan struct{}, 0),
		discardCount: 0,
	}
	go forwarder.loop()
	return forwarder
}

func (p *progressForwarderWithDiscarding) receive(message string) {
	// Try to push the message into the buffer, but otherwise just discard
	// it if the buffer is full
	select {
	case p.buffer <- message:
	default:
		p.discardCount++
	}
}

func (p *progressForwarderWithDiscarding) loop() {
	for {
		select {
		case msg := <-p.buffer:
			p.callback(status.StatusProvisioning, msg, nil)
		case <-p.stop:
			return
		}
	}
}

// Stop the forwarder, and report how many messages we discarded
func (p *progressForwarderWithDiscarding) Stop() int {
	close(p.stop)
	return p.discardCount
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
	forwarder := NewProgressForwarder(callback)
	err = manager.client.EnsureImageExists(series, forwarder.receive)
	if err != nil {
		err = errors.Annotatef(err, "failed to ensure LXD image")
		return
	}
	count := forwarder.Stop()
	if count > 0 {
		logger.Debugf("omitted %d progress messages", count)
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

	spec := lxdclient.InstanceSpec{
		Name:     name,
		Image:    manager.client.ImageNameForSeries(series),
		Metadata: metadata,
		Profiles: []string{
			"default",
		},
	}

	logger.Infof("starting instance %q (image %q)...", spec.Name, spec.Image)
	callback(status.StatusProvisioning, "Starting container", nil)
	_, err = manager.client.AddInstance(spec)
	if err != nil {
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
