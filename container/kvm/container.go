// Copyright 2013-2106 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux
// +build amd64 arm64 ppc64el

package kvm

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm/libvirt"
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

func (c *kvmContainer) Start(params StartParams) error {
	var srcFunc func() simplestreams.DataSource
	if params.ImageDownloadURL != "" {
		srcFunc = func() simplestreams.DataSource {
			return imagedownloads.NewDataSource(params.ImageDownloadURL)
		}
	}
	sp := syncParams{
		arch:    params.Arch,
		series:  params.Series,
		ftype:   FType,
		srcFunc: srcFunc,
	}
	logger.Debugf("synchronise images for %s %s %s", sp.arch, sp.series, params.ImageDownloadURL)
	if err := Sync(sp, nil); err != nil {
		if !errors.IsAlreadyExists(err) {
			return errors.Trace(err)
		}
		logger.Debugf("image already cached %s", err)
	}
	var bridge string
	var interfaces []libvirt.InterfaceInfo
	if params.Network != nil {
		if params.Network.NetworkType == container.BridgeNetwork {
			bridge = params.Network.Device
			for _, iface := range params.Network.Interfaces {
				interfaces = append(interfaces, iface)
			}
		} else {
			err := errors.New("Non-bridge network devices not yet supported")
			logger.Infof(err.Error())
			return err
		}
	}
	logger.Debugf("Create the machine %s", c.name)
	if err := CreateMachine(CreateMachineParams{
		Hostname:      c.name,
		Series:        params.Series,
		Arch:          params.Arch,
		UserDataFile:  params.UserDataFile,
		NetworkBridge: bridge,
		Memory:        params.Memory,
		CpuCores:      params.CpuCores,
		RootDisk:      params.RootDisk,
		Interfaces:    interfaces,
	}); err != nil {
		return err
	}

	logger.Debugf("Set machine %s to autostart", c.name)
	return AutostartMachine(c.name, run)
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
