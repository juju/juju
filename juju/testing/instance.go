// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

// FakeStateInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeStateInfo(machineId string) *mongo.MongoInfo {
	return &mongo.MongoInfo{
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

// AssertStartInstance is a test helper function that starts an instance with a
// plausible but invalid configuration, and checks that it succeeds.
func AssertStartInstance(
	c *gc.C, env environs.Environ, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstance(env, machineId)
	c.Assert(err, jc.ErrorIsNil)
	return inst, hc
}

// StartInstance is a test helper function that starts an instance with a plausible
// but invalid configuration, and returns the result of Environ.StartInstance.
func StartInstance(
	env environs.Environ, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.InterfaceInfo, error,
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
	c.Assert(err, jc.ErrorIsNil)
	return inst, hc
}

// StartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func StartInstanceWithConstraints(
	env environs.Environ, machineId string, cons constraints.Value,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.InterfaceInfo, error,
) {
	params := environs.StartInstanceParams{Constraints: cons}
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
	preferredSeries := config.PreferredSeries(env.Config())
	agentVersion, ok := env.Config().AgentVersion()
	if !ok {
		return nil, errors.New("missing agent version in model config")
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
		return nil, errors.Trace(err)
	}

	if params.ImageMetadata == nil {
		if err := SetImageMetadata(
			env,
			possibleTools.AllSeries(),
			possibleTools.Arches(),
			&params.ImageMetadata,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}

	machineNonce := "fake_nonce"
	stateInfo := FakeStateInfo(machineId)
	apiInfo := FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(
		machineId,
		machineNonce,
		imagemetadata.ReleasedStream,
		preferredSeries,
		"",
		true,
		stateInfo,
		apiInfo,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instanceConfig.Tags[tags.JujuModel] = env.Config().UUID()
	params.Tools = possibleTools
	params.InstanceConfig = instanceConfig
	return env.StartInstance(params)
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
