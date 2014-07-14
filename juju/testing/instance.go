// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/testing"
)

// FakeStateInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeStateInfo(machineId string) *authentication.MongoInfo {
	return &authentication.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{"0.1.2.3:1234"},
			CACert: testing.CACert,
		},
		Tag:      names.NewMachineTag(machineId),
		Password: "unimportant",
	}
}

// FakeAPIInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeAPIInfo(machineId string) *api.Info {
	return &api.Info{
		Addrs:    []string{"0.1.2.3:1234"},
		Tag:      names.NewMachineTag(machineId),
		Password: "unimportant",
		CACert:   testing.CACert,
	}
}

// WaitAddresses waits until the specified instance has addresses, and returns them.
func WaitInstanceAddresses(env environs.Environ, instId instance.Id) ([]network.Address, error) {
	for a := testing.LongAttempt.Start(); a.Next(); {
		insts, err := env.Instances([]instance.Id{instId})
		if err != nil {
			return nil, err
		}
		addresses, err := insts[0].Addresses()
		if err != nil {
			return nil, err
		}
		if len(addresses) > 0 {
			return addresses, nil
		}
	}
	return nil, fmt.Errorf("timed out trying to get addresses for %v", instId)
}

// AssertStartInstance is a test helper function that starts an instance with a
// plausible but invalid configuration, and checks that it succeeds.
func AssertStartInstance(
	c *gc.C, env environs.Environ, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstance(env, machineId)
	c.Assert(err, gc.IsNil)
	return inst, hc
}

// StartInstance is a test helper function that starts an instance with a plausible
// but invalid configuration, and returns the result of Environ.StartInstance.
func StartInstance(
	env environs.Environ, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	return StartInstanceWithConstraints(env, machineId, constraints.Value{})
}

// AssertStartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func AssertStartInstanceWithConstraints(
	c *gc.C, env environs.Environ, machineId string, cons constraints.Value,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstanceWithConstraints(env, machineId, cons)
	c.Assert(err, gc.IsNil)
	return inst, hc
}

// StartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func StartInstanceWithConstraints(
	env environs.Environ, machineId string, cons constraints.Value,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	return StartInstanceWithConstraintsAndNetworks(env, machineId, cons, nil)
}

// AssertStartInstanceWithNetworks is a test helper function that starts an
// instance with the given networks, and a plausible but invalid
// configuration, and returns the result of Environ.StartInstance.
func AssertStartInstanceWithNetworks(
	c *gc.C, env environs.Environ, machineId string, cons constraints.Value,
	networks []string,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstanceWithConstraintsAndNetworks(
		env, machineId, cons, networks)
	c.Assert(err, gc.IsNil)
	return inst, hc
}

// StartInstanceWithConstraintsAndNetworks is a test helper function that
// starts an instance with the given networks, and a plausible but invalid
// configuration, and returns the result of Environ.StartInstance.
func StartInstanceWithConstraintsAndNetworks(
	env environs.Environ, machineId string, cons constraints.Value,
	networks []string,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	params := environs.StartInstanceParams{Constraints: cons}
	return StartInstanceWithParams(
		env, machineId, params, networks)
}

// StartInstanceWithParams is a test helper function that starts an instance
// with the given parameters, and a plausible but invalid configuration, and
// returns the result of Environ.StartInstance. The provided params's
// MachineConfig and Tools field values will be ignored.
func StartInstanceWithParams(
	env environs.Environ, machineId string,
	params environs.StartInstanceParams,
	networks []string,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	series := config.PreferredSeries(env.Config())
	agentVersion, ok := env.Config().AgentVersion()
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing agent version in environment config")
	}
	possibleTools, err := tools.FindInstanceTools(
		env, agentVersion, series, params.Constraints.Arch,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	machineNonce := "fake_nonce"
	stateInfo := FakeStateInfo(machineId)
	apiInfo := FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(
		machineId, machineNonce,
		networks,
		stateInfo, apiInfo)
	params.Tools = possibleTools
	params.MachineConfig = machineConfig
	return env.StartInstance(params)
}
