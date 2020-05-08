// Copyright 2013-2106 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm/libvirt"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/juju/environs/simplestreams"
)

type kvmContainer struct {
	factory *containerFactory
	name    string
	// started is a three state boolean, true, false, or unknown
	// this allows for checking when we don't know, but using a
	// value if we already know it (like in the list situation).
	started *bool

	pathfinder func(string) (string, error)
	runCmd     runFunc
}

var _ Container = (*kvmContainer)(nil)

func (c *kvmContainer) Name() string {
	return c.name
}

// EnsureCachedImage ensures that a container image suitable for satisfying
// the input start parameters has been cached on disk.
func (c *kvmContainer) EnsureCachedImage(params StartParams) error {
	var srcFunc func() simplestreams.DataSource
	if params.ImageDownloadURL != "" {
		srcFunc = func() simplestreams.DataSource {
			return imagedownloads.NewDataSource(params.ImageDownloadURL)
		}
	}
	var fType = BIOSFType
	if params.Arch == arch.ARM64 {
		fType = UEFIFType
	}

	sp := syncParams{
		arch:    params.Arch,
		series:  params.Series,
		stream:  params.Stream,
		fType:   fType,
		srcFunc: srcFunc,
	}
	logger.Debugf("synchronise images for %s %s %s %s", sp.arch, sp.series, sp.stream, params.ImageDownloadURL)
	var callback ProgressCallback
	if params.StatusCallback != nil {
		callback = func(msg string) {
			_ = params.StatusCallback(status.Provisioning, msg, nil)
		}
	}
	if err := Sync(sp, nil, params.ImageDownloadURL, callback); err != nil {
		if !errors.IsAlreadyExists(err) {
			return errors.Trace(err)
		}
		logger.Debugf("image already cached %s", err)
	}
	return nil
}

// Start creates and starts a new container.
// It assumes that the backing image is already cached on disk.
func (c *kvmContainer) Start(params StartParams) error {
	var bridge string
	var interfaces []libvirt.InterfaceInfo
	if params.Network != nil {
		if params.Network.NetworkType == container.BridgeNetwork {
			bridge = params.Network.Device
			for _, iface := range params.Network.Interfaces {
				interfaces = append(interfaces, interfaceInfo{config: iface})
			}
		} else {
			err := errors.New("Non-bridge network devices not yet supported")
			logger.Infof(err.Error())
			return err
		}
	}
	logger.Debugf("create the machine %s", c.name)
	if params.StatusCallback != nil {
		_ = params.StatusCallback(status.Provisioning, "Creating instance", nil)
	}
	if err := CreateMachine(CreateMachineParams{
		Hostname:          c.name,
		Series:            params.Series,
		UserDataFile:      params.UserDataFile,
		NetworkConfigData: params.NetworkConfigData,
		NetworkBridge:     bridge,
		Memory:            params.Memory,
		CpuCores:          params.CpuCores,
		RootDisk:          params.RootDisk,
		Interfaces:        interfaces,
	}); err != nil {
		return err
	}

	logger.Debugf("Set machine %s to autostart", c.name)
	if params.StatusCallback != nil {
		_ = params.StatusCallback(status.Provisioning, "Starting instance", nil)
	}
	return AutostartMachine(c)
}

func (c *kvmContainer) Stop() error {
	if !c.IsRunning() {
		logger.Debugf("%s is already stopped", c.name)
		return nil
	}
	// Make started state unknown again.
	c.started = nil
	logger.Debugf("Stop %s", c)

	return DestroyMachine(c)
}

func (c *kvmContainer) IsRunning() bool {
	if c.started != nil {
		return *c.started
	}
	machines, err := ListMachines(run)
	if err != nil {
		return false
	}
	c.started = isRunning(machines[c.name])
	return *c.started
}

func (c *kvmContainer) String() string {
	return fmt.Sprintf("<KVM container %v>", *c)
}

type interfaceInfo struct {
	config corenetwork.InterfaceInfo
}

// MACAddress returns the embedded MacAddress value.
func (i interfaceInfo) MACAddress() string {
	return i.config.MACAddress
}

// InterfaceName returns the embedded InterfaceName value.
func (i interfaceInfo) InterfaceName() string {
	return i.config.InterfaceName
}

// ParentInterfaceName returns the embedded ParentInterfaceName value.
func (i interfaceInfo) ParentInterfaceName() string {
	return i.config.ParentInterfaceName
}
