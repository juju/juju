// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/rpc/params"
)

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
	externalCtrlService ExternalControllerService
	st                  ControllerConfigState
}

// NewControllerConfigAPI returns a new ControllerConfigAPI.
func NewControllerConfigAPI(
	st ControllerConfigState,
	externalCtrlService ExternalControllerService,
) *ControllerConfigAPI {
	return &ControllerConfigAPI{
		st:                  st,
		externalCtrlService: externalCtrlService,
	}
}

// ControllerConfig returns the controller's configuration.
func (s *ControllerConfigAPI) ControllerConfig() (params.ControllerConfigResult, error) {
	result := params.ControllerConfigResult{}
	config, err := s.st.LegacyControllerConfig()
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
	// First see if the requested model UUID is hosted by this controller.
	modelExists, err := s.st.ModelExists(modelTag.Id())
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}
	if modelExists {
		addrs, caCert, err := StateControllerInfo(s.st)
		if err != nil {
			return params.ControllerAPIInfoResult{}, errors.Trace(err)
		}
		return params.ControllerAPIInfoResult{
			Addresses: addrs,
			CACert:    caCert,
		}, nil
	}

	ctrl, err := s.externalCtrlService.ControllerForModel(ctx, modelTag.Id())
	if err == nil {
		return params.ControllerAPIInfoResult{
			Addresses: ctrl.Addrs,
			CACert:    ctrl.CACert,
		}, nil
	}
	if !errors.IsNotFound(err) {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}

	// The model may have been migrated from this controller to another.
	// If so, save the target as an external controller.
	// This will preserve cross-model relation consumers for models that were
	// on the same controller as migrated model, but not for consumers on other
	// controllers.
	// They will have to follow redirects and update their own relation data.
	mig, err := s.st.CompletedMigrationForModel(modelTag.Id())
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}
	target, err := mig.TargetInfo()
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}

	logger.Debugf("found migrated model on another controller, saving the information")
	err = s.externalCtrlService.UpdateExternalController(ctx, crossmodel.ControllerInfo{
		ControllerTag: target.ControllerTag,
		Alias:         target.ControllerAlias,
		Addrs:         target.Addrs,
		CACert:        target.CACert,
		ModelUUIDs:    []string{modelTag.Id()},
	})
	if err != nil {
		return params.ControllerAPIInfoResult{}, errors.Trace(err)
	}
	return params.ControllerAPIInfoResult{
		Addresses: target.Addrs,
		CACert:    target.CACert,
	}, nil
}

// StateControllerInfo returns the local controller details for the given State.
func StateControllerInfo(st controllerInfoState) (addrs []string, caCert string, _ error) {
	controllerConfig, err := st.LegacyControllerConfig()
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	addr, err := apiAddresses(controllerConfig, st)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	caCert, _ = controllerConfig.CACert()
	return addr, caCert, nil
}
