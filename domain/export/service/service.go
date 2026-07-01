// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

// Service provides the API for exporting model data.
type Service struct {
	st             State
	controllerInfo ControllerInfoState
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, controllerInfo ...ControllerInfoState) *Service {
	var info ControllerInfoState
	if len(controllerInfo) > 0 {
		info = controllerInfo[0]
	}
	return &Service{
		st:             st,
		controllerInfo: info,
	}
}
