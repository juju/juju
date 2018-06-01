// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// EnvironConfigGetter implements environs.EnvironConfigGetter
// in terms of a *state.State.
type cloudEnvironConfigGetter struct {
	Backend
	region string
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g cloudEnvironConfigGetter) CloudSpec() (environs.CloudSpec, error) {
	model, err := g.Model()
	if err != nil {
		return environs.CloudSpec{}, errors.Trace(err)
	}
	cloudName := model.Cloud()
	regionName := g.region
	credentialTag, _ := model.CloudCredential()
	return stateenvirons.CloudSpec(g.Backend, cloudName, regionName, credentialTag)
}

// InstanceTypes returns instance type information for the cloud and region
// in which the current model is deployed.
func (api *CloudAPI) InstanceTypes(cons params.CloudInstanceTypesConstraints) (params.InstanceTypesResults, error) {
	return instanceTypes(api, environs.GetEnviron, cons)
}

type environGetFunc func(st environs.EnvironConfigGetter, newEnviron environs.NewEnvironFunc) (environs.Environ, error)

func instanceTypes(api *CloudAPI,
	environGet environGetFunc,
	cons params.CloudInstanceTypesConstraints,
) (params.InstanceTypesResults, error) {
	m, err := api.ctlrBackend.Model()
	if err != nil {
		return params.InstanceTypesResults{}, errors.Trace(err)
	}

	result := make([]params.InstanceTypesResult, len(cons.Constraints))
	// TODO(perrito666) Cache the results to avoid excessive querying of the cloud.
	// TODO(perrito666) Add Region<>Cloud validation.
	for i, cons := range cons.Constraints {
		value := constraints.Value{}
		if cons.Constraints != nil {
			value = *cons.Constraints
		}
		backend := cloudEnvironConfigGetter{
			Backend: api.backend,
			region:  cons.CloudRegion,
		}
		cloudTag, err := names.ParseCloudTag(cons.CloudTag)
		if err != nil {
			result[i] = params.InstanceTypesResult{Error: common.ServerError(err)}
			continue
		}
		if m.Cloud() != cloudTag.Id() {
			result[i] = params.InstanceTypesResult{Error: common.ServerError(errors.NotValidf("asking %s cloud information to %s cloud", cloudTag.Id(), m.Cloud()))}
			continue
		}

		env, err := environGet(backend, environs.New)
		if err != nil {
			return params.InstanceTypesResults{}, errors.Trace(err)
		}

		itCons := common.NewInstanceTypeConstraints(
			env,
			api.callContext,
			value,
		)
		it, err := common.InstanceTypes(itCons)
		if err != nil {
			result[i] = params.InstanceTypesResult{Error: common.ServerError(err)}
			continue
		}
		result[i] = it
	}

	return params.InstanceTypesResults{Results: result}, nil
}
