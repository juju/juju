// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/version"
)

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// API represents the controller model operator facade.
type API struct {
	*common.APIAddresser
	*common.PasswordChanger

	auth      facade.Authorizer
	ctrlState CAASControllerState
	state     CAASModelOperatorState
}

// NewAPIFromContent creates a new controller model facade from the supplied
// context.
func NewAPIFromContext(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewAPI(authorizer, resources,
		stateShim{ctx.StatePool().SystemState()},
		stateShim{ctx.State()})
}

// NewAPI is alternative means of constructing a controller model facade.
func NewAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	ctrlSt CAASControllerState,
	st CAASModelOperatorState) (*API, error) {

	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		auth:            authorizer,
		APIAddresser:    common.NewAPIAddresser(ctrlSt, resources),
		PasswordChanger: common.NewPasswordChanger(st, common.AuthFuncForTagKind(names.ModelTagKind)),
		ctrlState:       ctrlSt,
		state:           st,
	}, nil
}

// ModelOperatorProvisioningInfo returns the information needed for provisioning
// a new model operator into a caas cluster.
func (a *API) ModelOperatorProvisioningInfo() (params.ModelOperatorInfo, error) {
	var result params.ModelOperatorInfo
	controllerConf, err := a.ctrlState.ControllerConfig()
	if err != nil {
		return result, err
	}

	model, err := a.state.Model()
	if err != nil {
		return result, errors.Trace(err)
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return result, errors.Trace(err)
	}

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return result, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in the model config %q",
				modelConfig.Name()))
	}

	apiAddresses, err := a.APIAddresses()
	if err != nil && apiAddresses.Error != nil {
		err = apiAddresses.Error
	}
	if err != nil {
		return result, errors.Annotate(err, "getting api addresses")
	}

	imageRepo := controllerConf.CAASImageRepo()
	imageInfo := params.DockerImageInfo{
		Username:      imageRepo.Username,
		Password:      imageRepo.Password,
		Email:         imageRepo.Email,
		Repository:    imageRepo.Repository,
		Auth:          imageRepo.Auth.Content(),
		IdentityToken: imageRepo.IdentityToken.Content(),
		RegistryToken: imageRepo.RegistryToken.Content(),
	}
	if imageInfo.RegistryPath, err = podcfg.GetJujuOCIImagePath(controllerConf,
		vers.ToPatch(), version.OfficialBuild); err != nil {
		return result, errors.Trace(err)
	}

	result = params.ModelOperatorInfo{
		APIAddresses: apiAddresses.Result,
		ImageDetails: imageInfo,
		Version:      vers,
	}
	return result, nil
}

// ModelUUID returns the model UUID that this facade is used to operate.
// It is implemented here directly as a result of removing it from
// embedded APIAddresser *without* bumping the facade version.
// It should be blanked when this facade version is next incremented.
func (a *API) ModelUUID() params.StringResult {
	return params.StringResult{Result: a.state.ModelUUID()}
}
