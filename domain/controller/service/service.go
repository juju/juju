// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// GetControllerModelUUID returns the model UUID of the controller model.
	GetControllerModelUUID(ctx context.Context) (model.UUID, error)

	// GetControllerAgentInfo returns the controller agent information needed by
	// the controller.
	GetControllerAgentInfo(ctx context.Context) (controller.ControllerAgentInfo, error)

	// GetModelNamespaces returns the model namespaces of all models in the
	// state.
	GetModelNamespaces(ctx context.Context) ([]string, error)
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

// ControllerModelUUID returns the model UUID of the controller model.
func (s *Service) ControllerModelUUID(ctx context.Context) (model.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetControllerModelUUID(ctx)
}

// GetControllerAgentInfo returns the controller agent information needed by the
// controller.
func (s *Service) GetControllerAgentInfo(ctx context.Context) (controller.ControllerAgentInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetControllerAgentInfo(ctx)
}

// GetModelNamespaces returns the model namespaces of all models in the
// state.
func (s *Service) GetModelNamespaces(ctx context.Context) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetModelNamespaces(ctx)
}
