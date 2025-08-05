// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package netplan

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/utils/scriptrunner"
)

var logger = loggo.GetLogger("juju.network.netplan")

// ActivationParams contains options to use when bridging interfaces
type ActivationParams struct {
	Clock     clock.Clock
	Devices   []DeviceToBridge
	RunPrefix string
	Directory string
	Timeout   time.Duration
}

// ActivationResult captures the result of actively bridging the
// interfaces using ifup/ifdown.
type ActivationResult struct {
	Stdout string
	Stderr string
	Code   int
}

// BridgeAndActivate will parse a set of netplan yaml files in a directory,
// create a new netplan config with the provided interfaces bridged
// bridged, then reconfigure the network using the ifupdown package
// for the new bridges.
func BridgeAndActivate(params ActivationParams) (*ActivationResult, error) {
	if len(params.Devices) == 0 {
		return nil, errors.Errorf("no devices specified")
	}

	netplan, err := ReadDirectory(params.Directory)

	if err != nil {
		return nil, err
	}

	for _, device := range params.Devices {
		deviceID, deviceType, err := netplan.FindDeviceByNameOrMAC(device.DeviceName, device.MACAddress)
		if err != nil {
			return nil, errors.Trace(err)
		}
		switch deviceType {
		case TypeEthernet:
			err = netplan.BridgeEthernetById(deviceID, device.BridgeName)
			if err != nil {
				return nil, err
			}
		case TypeBond:
			err = netplan.BridgeBondById(deviceID, device.BridgeName)
			if err != nil {
				return nil, err
			}
		case TypeVLAN:
			err = netplan.BridgeVLANById(deviceID, device.BridgeName)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.Errorf("unable to create bridge for %q, unknown device type %q", deviceID, deviceType)
		}
	}
	_, err = netplan.Write("")
	if err != nil {
		return nil, err
	}

	err = netplan.MoveYamlsToBak()
	if err != nil {
		_ = netplan.Rollback()
		return nil, err
	}

	environ := os.Environ()
	// TODO(wpk) 2017-06-21 Is there a way to verify that apply is finished?
	// https://bugs.launchpad.net/netplan/+bug/1701436
	command := fmt.Sprintf("%snetplan generate && netplan apply && sleep 10", params.RunPrefix)

	result, err := scriptrunner.RunCommand(command, environ, params.Clock, params.Timeout)
	if err != nil {
		_ = netplan.Rollback()
		return nil, errors.Annotatef(err, "activating bridge")
	}

	logger.Debugf("Netplan activation result %q %q %d", result.Stderr, result.Stdout, result.Code)
	activationResult := ActivationResult{
		Stderr: string(result.Stderr),
		Stdout: string(result.Stdout),
		Code:   result.Code,
	}

	if result.Code != 0 {
		_ = netplan.Rollback()
		return &activationResult, errors.Errorf("activating bridge: error code %d", result.Code)
	}
	return nil, nil
}
