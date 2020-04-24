// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

func fakeCallback(_ status.Status, _ string, _ map[string]interface{}) error {
	return nil
}

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
func WaitInstanceAddresses(
	env environs.Environ, ctx context.ProviderCallContext, instId instance.Id,
) (corenetwork.ProviderAddresses, error) {
	for a := testing.LongAttempt.Start(); a.Next(); {
		insts, err := env.Instances(ctx, []instance.Id{instId})
		if err != nil {
			return nil, err
		}
		addresses, err := insts[0].Addresses(ctx)
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
	c *gc.C, env environs.Environ, ctx context.ProviderCallContext, controllerUUID, machineId string,
) (
	instances.Instance, *instance.HardwareCharacteristics,
) {
	params := environs.StartInstanceParams{ControllerUUID: controllerUUID}
	err := FillInStartInstanceParams(env, machineId, true, &params)
	c.Assert(err, jc.ErrorIsNil)
	result, err := env.StartInstance(ctx, params)
	c.Assert(err, jc.ErrorIsNil)
	return result.Instance, result.Hardware
}

// AssertStartInstance is a test helper function that starts an instance with a
// plausible but invalid configuration, and checks that it succeeds.
func AssertStartInstance(
	c *gc.C, env environs.Environ, ctx context.ProviderCallContext, controllerUUID, machineId string,
) (
	instances.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstance(env, ctx, controllerUUID, machineId)
	c.Assert(err, jc.ErrorIsNil)
	return inst, hc
}

// StartInstance is a test helper function that starts an instance with a plausible
// but invalid configuration, and returns the result of Environ.StartInstance.
func StartInstance(
	env environs.Environ, ctx context.ProviderCallContext, controllerUUID, machineId string,
) (
	instances.Instance, *instance.HardwareCharacteristics, []corenetwork.InterfaceInfo, error,
) {
	return StartInstanceWithConstraints(env, ctx, controllerUUID, machineId, constraints.Value{})
}

// AssertStartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func AssertStartInstanceWithConstraints(
	c *gc.C, env environs.Environ, ctx context.ProviderCallContext,
	controllerUUID, machineId string, cons constraints.Value,
) (
	instances.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstanceWithConstraints(env, ctx, controllerUUID, machineId, cons)
	c.Assert(err, jc.ErrorIsNil)
	return inst, hc
}

// StartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func StartInstanceWithConstraints(
	env environs.Environ,
	ctx context.ProviderCallContext,
	controllerUUID, machineId string, cons constraints.Value,
) (
	instances.Instance, *instance.HardwareCharacteristics, []corenetwork.InterfaceInfo, error,
) {
	params := environs.StartInstanceParams{ControllerUUID: controllerUUID, Constraints: cons, StatusCallback: fakeCallback}
	result, err := StartInstanceWithParams(env, ctx, machineId, params)
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
	env environs.Environ, ctx context.ProviderCallContext,
	machineId string,
	params environs.StartInstanceParams,
) (
	*environs.StartInstanceResult, error,
) {
	if err := FillInStartInstanceParams(env, machineId, false, &params); err != nil {
		return nil, err
	}
	return env.StartInstance(ctx, params)
}

// FillInStartInstanceParams prepares the instance parameters for starting
// the instance.
func FillInStartInstanceParams(env environs.Environ, machineId string, isController bool, params *environs.StartInstanceParams) error {
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
	streams := tools.PreferredStreams(&agentVersion, env.Config().Development(), env.Config().AgentStream())
	possibleTools, err := tools.FindTools(env, -1, -1, streams, filter)
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
					Addrs:  []string{"localhost:1234"},
					CACert: "CA CERT\n" + testing.CACert,
				},
				Password: "mongosecret",
				Tag:      names.NewMachineTag(machineId),
			},
		}
		instanceConfig.Jobs = []model.MachineJob{model.JobHostUnits, model.JobManageModel}
	}
	cfg := env.Config()
	instanceConfig.Tags = instancecfg.InstanceTags(env.Config().UUID(), params.ControllerUUID, cfg, nil)
	params.Tools = possibleTools
	params.InstanceConfig = instanceConfig
	if params.StatusCallback == nil {
		params.StatusCallback = fakeCallback
	}
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
