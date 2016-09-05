// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

// FakeAPIInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeAPIInfo(machineId string) *api.Info {
	return &api.Info{
		Addrs:    []string{"0.1.2.3:17777"},
		Tag:      names.NewMachineTag(machineId),
		Password: "unimportant",
		CACert:   testing.CACert,
		ModelTag: testing.ModelTag,
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
	return nil, errors.Errorf("timed out trying to get addresses for %v", instId)
}

// AssertStartControllerInstance is a test helper function that starts a
// controller instance with a plausible but invalid configuration, and
// checks that it succeeds.
func AssertStartControllerInstance(
	c *gc.C, env environs.Environ, controllerUUID, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	params := environs.StartInstanceParams{ControllerUUID: controllerUUID}
	err := fillinStartInstanceParams(env, machineId, true, &params)
	c.Assert(err, jc.ErrorIsNil)
	result, err := env.StartInstance(params)
	c.Assert(err, jc.ErrorIsNil)
	return result.Instance, result.Hardware
}

// AssertStartInstance is a test helper function that starts an instance with a
// plausible but invalid configuration, and checks that it succeeds.
func AssertStartInstance(
	c *gc.C, env environs.Environ, controllerUUID, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstance(env, controllerUUID, machineId)
	c.Assert(err, jc.ErrorIsNil)
	return inst, hc
}

// StartInstance is a test helper function that starts an instance with a plausible
// but invalid configuration, and returns the result of Environ.StartInstance.
func StartInstance(
	env environs.Environ, controllerUUID, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.InterfaceInfo, error,
) {
	return StartInstanceWithConstraints(env, controllerUUID, machineId, constraints.Value{})
}

// AssertStartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func AssertStartInstanceWithConstraints(
	c *gc.C, env environs.Environ, controllerUUID, machineId string, cons constraints.Value,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstanceWithConstraints(env, controllerUUID, machineId, cons)
	c.Assert(err, jc.ErrorIsNil)
	return inst, hc
}

// StartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func StartInstanceWithConstraints(
	env environs.Environ, controllerUUID, machineId string, cons constraints.Value,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.InterfaceInfo, error,
) {
	params := environs.StartInstanceParams{ControllerUUID: controllerUUID, Constraints: cons}
	result, err := StartInstanceWithParams(env, machineId, params)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	return result.Instance, result.Hardware, result.NetworkInfo, nil
}

// StartInstanceWithParams is a test helper function that starts an instance
// with the given parameters, and a plausible but invalid configuration, and
// returns the result of Environ.StartInstance. The provided params's
// InstanceConfig and Tools field values will be ignored.
func StartInstanceWithParams(
	env environs.Environ, machineId string,
	params environs.StartInstanceParams,
) (
	*environs.StartInstanceResult, error,
) {
	if err := fillinStartInstanceParams(env, machineId, false, &params); err != nil {
		return nil, err
	}
	return env.StartInstance(params)
}

func fillinStartInstanceParams(env environs.Environ, machineId string, isController bool, params *environs.StartInstanceParams) error {
	if params.ControllerUUID == "" {
		return errors.New("missing controller UUID in start instance parameters")
	}
	preferredSeries := config.PreferredSeries(env.Config())
	agentVersion, ok := env.Config().AgentVersion()
	if !ok {
		return errors.New("missing agent version in model config")
	}
	filter := coretools.Filter{
		Number: agentVersion,
		Series: preferredSeries,
	}
	if params.Constraints.Arch != nil {
		filter.Arch = *params.Constraints.Arch
	}
	stream := tools.PreferredStream(&agentVersion, env.Config().Development(), env.Config().AgentStream())
	possibleTools, err := tools.FindTools(env, -1, -1, stream, filter)
	if err != nil {
		return errors.Trace(err)
	}

	if params.ImageMetadata == nil {
		if err := SetImageMetadata(
			env,
			possibleTools.AllSeries(),
			possibleTools.Arches(),
			&params.ImageMetadata,
		); err != nil {
			return errors.Trace(err)
		}
	}

	machineNonce := "fake_nonce"
	apiInfo := FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(
		testing.ControllerTag,
		machineId,
		machineNonce,
		imagemetadata.ReleasedStream,
		preferredSeries,
		apiInfo,
	)
	if err != nil {
		return errors.Trace(err)
	}
	if isController {
		instanceConfig.Controller = &instancecfg.ControllerConfig{
			Config: testing.FakeControllerConfig(),
			MongoInfo: &mongo.MongoInfo{
				Info: mongo.Info{
					Addrs:  []string{"127.0.0.1:1234"},
					CACert: "CA CERT\n" + testing.CACert,
				},
				Password: "mongosecret",
				Tag:      names.NewMachineTag(machineId),
			},
		}
		instanceConfig.Jobs = []multiwatcher.MachineJob{multiwatcher.JobHostUnits, multiwatcher.JobManageModel}
	}
	cfg := env.Config()
	instanceConfig.Tags = instancecfg.InstanceTags(env.Config().UUID(), params.ControllerUUID, cfg, nil)
	params.Tools = possibleTools
	params.InstanceConfig = instanceConfig
	return nil
}

func SetImageMetadata(env environs.Environ, series, arches []string, out *[]*imagemetadata.ImageMetadata) error {
	hasRegion, ok := env.(simplestreams.HasRegion)
	if !ok {
		return nil
	}
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return errors.Trace(err)
	}
	region, err := hasRegion.Region()
	if err != nil {
		return errors.Trace(err)
	}
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: region,
		Series:    series,
		Arches:    arches,
		Stream:    env.Config().ImageStream(),
	})
	imageMetadata, _, err := imagemetadata.Fetch(sources, imageConstraint)
	if err != nil {
		return errors.Trace(err)
	}
	*out = imageMetadata
	return nil
}
