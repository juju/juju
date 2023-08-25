// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/network/debinterfaces"
	"github.com/juju/juju/internal/network/netplan"
)

// Bridger creates network bridges to support addressable containers.
type Bridger interface {
	// Bridge turns existing devices into bridged devices.
	Bridge(devices []DeviceToBridge, reconfigureDelay int) error
}

type etcNetworkInterfacesBridger struct {
	Clock    clock.Clock
	DryRun   bool
	Filename string
	Timeout  time.Duration
}

var _ Bridger = (*etcNetworkInterfacesBridger)(nil)

func (b *etcNetworkInterfacesBridger) Bridge(devices []DeviceToBridge, reconfigureDelay int) error {
	devicesMap := make(map[string]string)
	for _, k := range devices {
		devicesMap[k.DeviceName] = k.BridgeName
	}
	params := debinterfaces.ActivationParams{
		Clock:            clock.WallClock,
		Filename:         b.Filename,
		Devices:          devicesMap,
		ReconfigureDelay: reconfigureDelay,
		Timeout:          b.Timeout,
		DryRun:           b.DryRun,
	}

	result, err := debinterfaces.BridgeAndActivate(params)
	if err != nil {
		return errors.Errorf("bridge activation error: %s", err)
	}
	if result != nil {
		logger.Infof("bridgescript result=%v", result.Code)
		if result.Code != 0 {
			logger.Errorf("bridgescript stdout\n%s\n", result.Stdout)
			logger.Errorf("bridgescript stderr\n%s\n", result.Stderr)
			return errors.Errorf("bridgescript failed: %s", string(result.Stderr))
		}
		logger.Tracef("bridgescript stdout\n%s\n", result.Stdout)
		logger.Tracef("bridgescript stderr\n%s\n", result.Stderr)
	} else {
		logger.Infof("bridgescript returned nothing")
	}

	return nil
}

func newEtcNetworkInterfacesBridger(clock clock.Clock, timeout time.Duration, filename string, dryRun bool) Bridger {
	return &etcNetworkInterfacesBridger{
		Clock:    clock,
		DryRun:   dryRun,
		Filename: filename,
		Timeout:  timeout,
	}
}

// DefaultEtcNetworkInterfacesBridger returns a Bridger instance that
// can parse an interfaces(5) to transform existing devices into
// bridged devices.
func DefaultEtcNetworkInterfacesBridger(timeout time.Duration, filename string) (Bridger, error) {
	return newEtcNetworkInterfacesBridger(clock.WallClock, timeout, filename, false), nil
}

type netplanBridger struct {
	Clock     clock.Clock
	Directory string
	Timeout   time.Duration
}

var _ Bridger = (*netplanBridger)(nil)

func (b *netplanBridger) Bridge(devices []DeviceToBridge, reconfigureDelay int) error {
	npDevices := make([]netplan.DeviceToBridge, len(devices))
	for i, device := range devices {
		npDevices[i] = netplan.DeviceToBridge(device)
	}
	params := netplan.ActivationParams{
		Clock:     clock.WallClock,
		Directory: b.Directory,
		Devices:   npDevices,
		Timeout:   b.Timeout,
	}

	result, err := netplan.BridgeAndActivate(params)
	if err != nil {
		return errors.Errorf("bridge activation error: %s", err)
	}
	if result != nil {
		logger.Infof("bridger result=%v", result.Code)
		if result.Code != 0 {
			logger.Errorf("bridger stdout\n%s\n", result.Stdout)
			logger.Errorf("bridger stderr\n%s\n", result.Stderr)
			return errors.Errorf("bridger failed: %s", result.Stderr)
		}
		logger.Tracef("bridger stdout\n%s\n", result.Stdout)
		logger.Tracef("bridger stderr\n%s\n", result.Stderr)
	} else {
		logger.Infof("bridger returned nothing")
	}

	return nil
}

func newNetplanBridger(clock clock.Clock, timeout time.Duration, directory string) Bridger {
	return &netplanBridger{
		Clock:     clock,
		Directory: directory,
		Timeout:   timeout,
	}
}

// DefaultNetplanBridger returns a Bridger instance that can parse a set
// of netplan yaml files to transform existing devices into bridged devices.
func DefaultNetplanBridger(timeout time.Duration, directory string) (Bridger, error) {
	return newNetplanBridger(clock.WallClock, timeout, directory), nil
}
