// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// InstanceTypes returns instance type information for the cloud and region
// in which the current model is deployed.
func (mm *MachineManagerAPI) InstanceTypes(cons params.ModelInstanceTypesConstraints) (params.InstanceTypesResults, error) {
	return instanceTypes(mm, environs.GetEnviron, cons)
}

type environGetFunc func(st environs.EnvironConfigGetter, newEnviron environs.NewEnvironFunc) (environs.Environ, error)

func instanceTypes(mm *MachineManagerAPI,
	getEnviron environGetFunc,
	cons params.ModelInstanceTypesConstraints,
) (params.InstanceTypesResults, error) {
	model, err := mm.st.GetModel(mm.st.ModelTag())
	if err != nil {
		return params.InstanceTypesResults{}, errors.Trace(err)
	}

	cloudSpec := func(tag names.ModelTag) (environs.CloudSpec, error) {
		cloudName := model.Cloud()
		regionName := model.CloudRegion()
		credentialTag, _ := model.CloudCredential()
		return stateenvirons.CloudSpec(mm.st, cloudName, regionName, credentialTag)
	}
	backend := common.EnvironConfigGetterFuncs{
		CloudSpecFunc:   cloudSpec,
		ModelConfigFunc: model.Config,
	}

	env, err := getEnviron(backend, environs.New)
	result := make([]params.InstanceTypesResult, len(cons.Constraints))
	// TODO(perrito666) Cache the results to avoid excessive querying of the cloud.
	for i, c := range cons.Constraints {
		value := constraints.Value{}
		if c.Value != nil {
			value = *c.Value
		}
		itCons := common.NewInstanceTypeConstraints(env, value)
		it, err := common.InstanceTypes(itCons)
		if err != nil {
			it = params.InstanceTypesResult{Error: common.ServerError(err)}
		}
		result[i] = it
	}

	return params.InstanceTypesResults{Results: result}, nil
}
