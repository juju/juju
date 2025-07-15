// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigService is an interface that provides the controller
// configuration for the model.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelService is an interface that provides information about hosted models.
type ModelService interface {
	// CheckModelExists checks if a model exists within the controller. True or
	// false is returned indiciating of the model exists.
	CheckModelExists(ctx context.Context, modelUUID coremodel.UUID) (bool, error)

	// ModelRedirection returns redirection information for the current model. If it
	// is not redirected, [modelmigrationerrors.ModelNotRedirected] is returned.
	ModelRedirection(ctx context.Context, modelUUID coremodel.UUID) (model.ModelRedirection, error)
}

// APIHostPortsForAgentsGetter represents a way to get controller api addresses.
type APIHostPortsForAgentsGetter interface {
	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
}

// ExternalControllerService defines the methods that the controller
// facade needs from the controller state.
type ExternalControllerService interface {
	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)

	// UpdateExternalController persists the input controller
	// record.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error
}

// ControllerConfigAPI implements two common methods for use by various
// facades - eg Provisioner and ControllerConfig.
type ControllerConfigAPI struct {
	controllerConfigService     ControllerConfigService
	apiHostPortsForAgentsGetter APIHostPortsForAgentsGetter
	externalControllerService   ExternalControllerService
	modelService                ModelService
}

// NewControllerConfigAPI returns a new ControllerConfigAPI.
func NewControllerConfigAPI(
	controllerConfigService ControllerConfigService,
	apiHostPortsForAgentsGetter APIHostPortsForAgentsGetter,
	externalControllerService ExternalControllerService,
	modelService ModelService,
) *ControllerConfigAPI {
	return &ControllerConfigAPI{
		controllerConfigService:     controllerConfigService,
		apiHostPortsForAgentsGetter: apiHostPortsForAgentsGetter,
		externalControllerService:   externalControllerService,
		modelService:                modelService,
	}
}

// ControllerConfig returns the controller's configuration.
func (s *ControllerConfigAPI) ControllerConfig(ctx context.Context) (params.ControllerConfigResult, error) {
	result := params.ControllerConfigResult{}
	config, err := s.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, err
	}
	result.Config = params.ControllerConfig(config)
	return result, nil
}

// ControllerAPIInfoForModels returns the controller api connection details for the specified models.
func (s *ControllerConfigAPI) ControllerAPIInfoForModels(ctx context.Context, args params.Entities) (params.ControllerAPIInfoResults, error) {
	var result params.ControllerAPIInfoResults
	result.Results = make([]params.ControllerAPIInfoResult, len(args.Entities))
	for i, entity := range args.Entities {
		info, err := s.getModelControllerInfo(ctx, entity)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = info
	}
	return result, nil
}

// GetModelControllerInfo returns the external controller details for the specified model.
func (s *ControllerConfigAPI) getModelControllerInfo(ctx context.Context, model params.Entity) (params.ControllerAPIInfoResult, error) {
	modelTag, err := names.ParseModelTag(model.Tag)
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}
	modelUUID := coremodel.UUID(modelTag.Id())
	// First see if the requested model UUID is hosted by this controller.
	modelExists, err := s.modelService.CheckModelExists(ctx, modelUUID)
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}
	if modelExists {
		addrs, caCert, err := ControllerAPIInfo(ctx, s.controllerConfigService, s.apiHostPortsForAgentsGetter)
		if err != nil {
			return params.ControllerAPIInfoResult{}, errors.Trace(err)
		}
		return params.ControllerAPIInfoResult{
			Addresses: addrs,
			CACert:    caCert,
		}, nil
	}

	ctrl, err := s.externalControllerService.ControllerForModel(ctx, modelTag.Id())
	if err == nil {
		return params.ControllerAPIInfoResult{
			Addresses: ctrl.Addrs,
			CACert:    ctrl.CACert,
		}, nil
	}
	if !errors.Is(err, errors.NotFound) {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}

	// The model may have been migrated from this controller to another.
	// If so, save the target as an external controller.
	// This will preserve cross-model relation consumers for models that were
	// on the same controller as migrated model, but not for consumers on other
	// controllers.
	// They will have to follow redirects and update their own relation data.
	modelRedirection, err := s.modelService.ModelRedirection(ctx, modelUUID)
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}

	logger.Debugf(ctx, "found migrated model on another controller, saving the information")
	err = s.externalControllerService.UpdateExternalController(ctx, crossmodel.ControllerInfo{
		ControllerUUID: modelRedirection.ControllerUUID,
		Alias:          modelRedirection.ControllerAlias,
		Addrs:          modelRedirection.Addresses,
		CACert:         modelRedirection.CACert,
		ModelUUIDs:     []string{modelUUID.String()},
	})
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}
	return params.ControllerAPIInfoResult{
		Addresses: modelRedirection.Addresses,
		CACert:    modelRedirection.CACert,
	}, nil
}

// ControllerAPIInfo returns the local controller details for the given State.
func ControllerAPIInfo(
	ctx context.Context,
	controllerConfigService ControllerConfigService,
	apiHostPortsGetter APIHostPortsForAgentsGetter,
) ([]string, string, error) {
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	addrs, err := apiHostPortsGetter.GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	var caCert string
	caCert, _ = controllerConfig.CACert()
	return addrs, caCert, nil
}
