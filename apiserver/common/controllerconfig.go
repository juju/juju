// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

// ControllerConfigAPI implements two common methods for use by various
// facades - eg Provisioner and ControllerConfig.
type ControllerConfigAPI struct {
	st state.ControllerAccessor
}

// NewStateControllerConfig returns a new NewControllerConfigAPI.
func NewStateControllerConfig(st *state.State) *ControllerConfigAPI {
	return NewControllerConfig(&controllerStateShim{st})
}

// NewControllerConfig returns a new NewControllerConfigAPI.
func NewControllerConfig(st state.ControllerAccessor) *ControllerConfigAPI {
	return &ControllerConfigAPI{
		st: st,
	}
}

// ControllerConfig returns the controller's configuration.
func (s *ControllerConfigAPI) ControllerConfig() (params.ControllerConfigResult, error) {
	result := params.ControllerConfigResult{}
	config, err := s.st.ControllerConfig()
	if err != nil {
		return result, err
	}
	result.Config = params.ControllerConfig(config)
	return result, nil
}

// ControllerAPIInfoForModels returns the controller api connection details for the specified models.
func (s *ControllerConfigAPI) ControllerAPIInfoForModels(args params.Entities) (params.ControllerAPIInfoResults, error) {
	var result params.ControllerAPIInfoResults
	result.Results = make([]params.ControllerAPIInfoResult, len(args.Entities))
	for i, entity := range args.Entities {
		modelTag, err := names.ParseModelTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		addrs, caCert, err := s.st.ControllerInfo(modelTag.Id())
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		result.Results[i].Addresses = addrs
		result.Results[i].CACert = caCert
	}
	return result, nil
}

type controllerStateShim struct {
	*state.State
}

// ControllerInfo returns the external controller details for the specified model.
func (s *controllerStateShim) ControllerInfo(modelUUID string) (addrs []string, CACert string, _ error) {
	// First see if the requested model UUID is hosted by this controller.
	modelExists, err := s.State.ModelExists(modelUUID)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if modelExists {
		return StateControllerInfo(s.State)
	}

	ec := state.NewExternalControllers(s.State)
	ctrl, err := ec.ControllerForModel(modelUUID)
	if err == nil {
		return ctrl.ControllerInfo().Addrs, ctrl.ControllerInfo().CACert, nil
	}
	if !errors.IsNotFound(err) {
		return nil, "", errors.Trace(err)
	}

	// The model may have been migrated from this controller to another.
	// If so, save the target as an external controller.
	// This will preserve cross-model relation consumers for models that were
	// on the same controller as migrated model, but not for consumers on other
	// controllers.
	// They will have to follow redirects and update their own relation data.
	mig, err := s.State.CompletedMigrationForModel(modelUUID)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	target, err := mig.TargetInfo()
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	logger.Debugf("found migrated model on another controller, saving the information")
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: target.ControllerTag,
		Alias:         target.ControllerAlias,
		Addrs:         target.Addrs,
		CACert:        target.CACert,
	}, modelUUID)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return target.Addrs, target.CACert, nil
}

// StateControllerInfo returns the local controller details for the given State.
func StateControllerInfo(st *state.State) (addrs []string, caCert string, _ error) {
	addr, err := apiAddresses(st)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	caCert, _ = controllerConfig.CACert()
	return addr, caCert, nil
}
