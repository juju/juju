// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/stateenvirons"
)

// EnvironConfigGetter implements environs.EnvironConfigGetter
// in terms of a *state.State.
type cloudEnvironConfigGetter struct {
	Backend
	region         string
	controllerUUID string
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g cloudEnvironConfigGetter) CloudSpec() (environscloudspec.CloudSpec, error) {
	model, err := g.Model()
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	return stateenvirons.CloudSpecForModel(model)
}

func (g cloudEnvironConfigGetter) ControllerUUID() string {
	return g.controllerUUID
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
	_, callContext, releaser, err := api.pool.GetModelCallContext(m.UUID())
	if err != nil {
		return params.InstanceTypesResults{}, errors.Trace(err)
	}
	defer releaser()

	ctrlCfg, err := api.backend.ControllerConfig()
	if err != nil {
		return params.InstanceTypesResults{}, errors.Annotate(err, "getting controller config")
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
			Backend:        api.backend,
			region:         cons.CloudRegion,
			controllerUUID: ctrlCfg.ControllerUUID(),
		}
		cloudTag, err := names.ParseCloudTag(cons.CloudTag)
		if err != nil {
			result[i] = params.InstanceTypesResult{Error: apiservererrors.ServerError(err)}
			continue
		}
		if m.CloudName() != cloudTag.Id() {
			result[i] = params.InstanceTypesResult{Error: apiservererrors.ServerError(errors.NotValidf("asking %s cloud information to %s cloud", cloudTag.Id(), m.CloudName()))}
			continue
		}

		env, err := environGet(backend, environs.New)
		if err != nil {
			return params.InstanceTypesResults{}, errors.Trace(err)
		}
		itCons := common.NewInstanceTypeConstraints(
			env,
			callContext,
			value,
		)
		it, err := common.InstanceTypes(itCons)
		if err != nil {
			result[i] = params.InstanceTypesResult{Error: apiservererrors.ServerError(err)}
			continue
		}
		result[i] = it
	}

	return params.InstanceTypesResults{Results: result}, nil
}
