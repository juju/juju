// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	jujucontroller "github.com/juju/juju/controller"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	ControllerConfig(context.Context) (jujucontroller.Config, error)
	UpdateControllerConfig(ctx context.Context, updateAttrs map[string]interface{}, removeAttrs []string) error
	checkUpdateControllerConfig(ctx context.Context, name string) error
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// ControllerConfig returns the controller config.
func (s *Service) ControllerConfig(ctx context.Context) (jujucontroller.Config, error) {
	return nil, nil
}

// UpdateControllerConfig updates the controller config.
func (s *Service) UpdateControllerConfig(ctx context.Context, updateAttrs map[string]interface{}, removeAttrs []string) error {
	return nil
}

// checkUpdateControllerConfig checks the update controller config.
func (s *Service) checkUpdateControllerConfig(ctx context.Context, name string) error {
	return nil
}
