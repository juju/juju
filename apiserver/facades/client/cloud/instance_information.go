// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

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
	CloudService      common.CloudService
	CredentialService common.CredentialService
	region            string
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g cloudEnvironConfigGetter) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	model, err := g.Model()
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	return stateenvirons.CloudSpecForModel(ctx, model, g.CloudService, g.CredentialService)
}

// InstanceTypes returns instance type information for the cloud and region
// in which the current model is deployed.
func (api *CloudAPI) InstanceTypes(cons params.CloudInstanceTypesConstraints) (params.InstanceTypesResults, error) {
	return instanceTypes(context.TODO(), api, environs.GetEnviron, cons)
}

type environGetFunc func(context.Context, environs.EnvironConfigGetter, environs.NewEnvironFunc) (environs.Environ, error)

func instanceTypes(
	ctx context.Context,
	api *CloudAPI,
	environGet environGetFunc,
	cons params.CloudInstanceTypesConstraints,
) (params.InstanceTypesResults, error) {
	m, err := api.ctlrBackend.Model()
	if err != nil {
		return params.InstanceTypesResults{}, errors.Trace(err)
	}
	_, callContext, err := api.pool.GetModelCallContext(m.UUID())
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
			Backend:           api.backend,
			CloudService:      api.cloudService,
			CredentialService: api.credentialService,
			region:            cons.CloudRegion,
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

		env, err := environGet(ctx, backend, environs.New)
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
