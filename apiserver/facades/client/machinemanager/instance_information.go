// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/stateenvirons"
)

// InstanceTypes returns instance type information for the cloud and region
// in which the current model is deployed.
func (mm *MachineManagerAPI) InstanceTypes(cons params.ModelInstanceTypesConstraints) (params.InstanceTypesResults, error) {
	return instanceTypes(mm, environs.GetEnviron, cons)
}

type environGetFunc func(st environs.EnvironConfigGetter, controllerUUID string, newEnviron environs.NewEnvironFunc) (environs.Environ, error)

func instanceTypes(mm *MachineManagerAPI,
	getEnviron environGetFunc,
	cons params.ModelInstanceTypesConstraints,
) (params.InstanceTypesResults, error) {
	model, err := mm.st.Model()
	if err != nil {
		return params.InstanceTypesResults{}, errors.Trace(err)
	}

	cloudSpec := func() (environscloudspec.CloudSpec, error) {
		return stateenvirons.CloudSpecForModel(model)
	}
	backend := common.EnvironConfigGetterFuncs{
		CloudSpecFunc:   cloudSpec,
		ModelConfigFunc: model.Config,
	}

	env, err := getEnviron(backend, model.ControllerUUID(), environs.New)
	if err != nil {
		return params.InstanceTypesResults{}, errors.Trace(err)
	}
	result := make([]params.InstanceTypesResult, len(cons.Constraints))
	// TODO(perrito666) Cache the results to avoid excessive querying of the cloud.
	for i, c := range cons.Constraints {
		value := constraints.Value{}
		if c.Value != nil {
			value = *c.Value
		}
		itCons := common.NewInstanceTypeConstraints(
			env,
			mm.callContext,
			value,
		)
		it, err := common.InstanceTypes(itCons)
		if err != nil {
			it = params.InstanceTypesResult{Error: apiservererrors.ServerError(err)}
		}
		result[i] = it
	}

	return params.InstanceTypesResults{Results: result}, nil
}
