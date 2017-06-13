package netplan

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/utils/scriptrunner"
)

var logger = loggo.GetLogger("juju.network.netplan")

const (
	RunModeRegular = iota
	RunModeSuccess = iota
	RunModeFailure = iota
	RunModeTimeout = iota
)

// ActivationParams contains options to use when bridging interfaces
type ActivationParams struct {
	Clock     clock.Clock
	Devices   []DeviceToBridge
	RunMode   int
	Directory string
	Timeout   time.Duration
}

// ActivationResult captures the result of actively bridging the
// interfaces using ifup/ifdown.
type ActivationResult struct {
	Stdout []byte
	Stderr []byte
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
		var deviceId string
		if device.MACAddress != "" {
			deviceId, err = netplan.FindEthernetByMAC(device.MACAddress)
			if errors.IsNotFound(err) {
				logger.Debugf("Trying to find ethernet by MAC %q", err)
				deviceId, err = netplan.FindEthernetByName(device.DeviceName)
			}
			if err != nil {
				return nil, err
			}
		}
		err = netplan.BridgeEthernetById(deviceId, device.BridgeName)
		if err != nil {
			return nil, err
		}
	}
	_, err = netplan.Write("")
	if err != nil {
		return nil, err
	}

	err = netplan.MoveYamlsToBak()
	if err != nil {
		netplan.Rollback()
		return nil, err
	}

	environ := os.Environ()
	// TODO(wpk) 2017-06-21 Is there a way to verify that apply is finished?
	command := "netplan generate && netplan apply && sleep 3"
	switch params.RunMode {
	case RunModeSuccess:
		command = "true"
	case RunModeFailure:
		command = "echo -n This is stderr >&2 && echo -n This is stdout && false"
	case RunModeTimeout:
		command = "sleep 10000"
	}

	result, err := scriptrunner.RunCommand(command, environ, params.Clock, params.Timeout)

	activationResult := ActivationResult{
		Stderr: result.Stderr,
		Stdout: result.Stdout,
		Code:   result.Code,
	}

	logger.Debugf("Netplan activation result %q %q %d", result.Stderr, result.Stdout, result.Code)

	if err != nil {
		netplan.Rollback()
		return &activationResult, errors.Errorf("bridge activation error: %s", err)
	}
	if result.Code != 0 {
		netplan.Rollback()
		return &activationResult, errors.Errorf("bridge activation error code %d", result.Code)
	}
	return nil, nil
}
