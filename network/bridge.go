// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/network/debinterfaces"
	"github.com/juju/utils/clock"
)

// Bridger creates network bridges to support addressable containers.
type Bridger interface {
	// Turns existing devices into bridged devices.
	// TODO(frobware) - we may want a different type to encompass
	// and reflect how bridging should be done vis-a-vis what
	// needs to be bridged.
	Bridge(devices []DeviceToBridge, reconfigureDelay int) error
}

type etcNetworkInterfacesBridger struct {
	Clock    clock.Clock
	DryRun   bool
	Filename string
	Timeout  time.Duration
}

var _ Bridger = (*etcNetworkInterfacesBridger)(nil)

func printParseError(err error) {
	if pe, ok := err.(*debinterfaces.ParseError); ok {
		fmt.Printf("error: %q:%d: %s: %s\n", pe.Filename, pe.LineNum, pe.Line, pe.Message)
	} else {
		fmt.Printf("error: %v\n", err)
	}
}

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
		return errors.Errorf("bridge activaction error: %s", err)
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
		logger.Infof("bridgecript returned nothing")
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
