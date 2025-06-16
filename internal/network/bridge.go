// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/network/netplan"
)

// Bridger creates network bridges to support addressable containers.
type Bridger interface {
	// Bridge turns existing devices into bridged devices.
	Bridge(devices []network.DeviceToBridge) error
}

type netplanBridger struct {
	Clock     clock.Clock
	Directory string
	Timeout   time.Duration
}

var _ Bridger = (*netplanBridger)(nil)

func (b *netplanBridger) Bridge(devices []network.DeviceToBridge) error {
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

	ctx := context.TODO()
	if result != nil {
		logger.Infof(ctx, "bridger result=%v", result.Code)
		if result.Code != 0 {
			logger.Errorf(ctx, "bridger stdout\n%s\n", result.Stdout)
			logger.Errorf(ctx, "bridger stderr\n%s\n", result.Stderr)
			return errors.Errorf("bridger failed: %s", result.Stderr)
		}
		logger.Tracef(ctx, "bridger stdout\n%s\n", result.Stdout)
		logger.Tracef(ctx, "bridger stderr\n%s\n", result.Stderr)
	} else {
		logger.Infof(ctx, "bridger returned nothing")
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
