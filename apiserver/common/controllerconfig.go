// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
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
		ec, err := s.st.ControllerInfo(modelTag.Id())
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		result.Results[i].Addresses = ec.ControllerInfo().Addrs
		result.Results[i].CACert = ec.ControllerInfo().CACert
	}
	return result, nil
}

type controllerStateShim struct {
	*state.State
}

// ControllerInfo returns the external controller details for the specified model.
func (s *controllerStateShim) ControllerInfo(modelUUID string) (state.ExternalController, error) {
	ec := state.NewExternalControllers(s.State)
	return ec.ControllerForModel(modelUUID)
}
