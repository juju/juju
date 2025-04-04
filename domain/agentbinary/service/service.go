// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/internal/errors"
)

// AgentBinaryState describes the interface that the agent binary state must
// implement. It is used to list agent binaries.
type AgentBinaryState interface {
	// ListAgentBinaries lists all agent binaries in the state.
	// It returns a slice of agent binary metadata.
	// An empty slice is returned if no agent binaries are found.
	ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error)
}

// AgentBinaryService provides the API for working with agent binaries.
// It is used to list agent binaries from the controller and model states.
// It merges the two lists of agent binaries, with the model agent binaries
// taking precedence over the controller agent binaries.
// The service is used to provide a unified view of the agent binaries
// across the controller and model states.
type AgentBinaryService struct {
	controllerState AgentBinaryState
	modelState      AgentBinaryState
}

// NewAgentBinaryService returns a new instance of AgentBinaryService.
// It takes two states: the controller state and the model state to aggregate the
// agent binaries from both states.
func NewAgentBinaryService(
	controllerState AgentBinaryState,
	modelState AgentBinaryState,
) *AgentBinaryService {
	return &AgentBinaryService{
		controllerState: controllerState,
		modelState:      modelState,
	}
}

// ListAgentBinaries lists all agent binaries in the controller and model states.
// It merges the two lists of agent binaries, with the model agent binaries
// taking precedence over the controller agent binaries.
// It returns a slice of agent binary metadata.
// An empty slice is returned if no agent binaries are found.
func (s *AgentBinaryService) ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error) {
	controllerAgentBinaries, err := s.controllerState.ListAgentBinaries(ctx)
	if err != nil {
		return nil, errors.Errorf("listing agent binaries from controller state: %w", err)
	}
	modelAgentBinaries, err := s.modelState.ListAgentBinaries(ctx)
	if err != nil {
		return nil, errors.Errorf("listing agent binaries from model state: %w", err)
	}

	// Merge the two lists of agent binaries. The model agent binaries
	// take precedence over the controller agent binaries.
	allAgentBinaries := make(map[string]agentbinary.Metadata)
	for _, ab := range controllerAgentBinaries {
		allAgentBinaries[ab.SHA256] = ab
	}
	for _, ab := range modelAgentBinaries {
		allAgentBinaries[ab.SHA256] = ab
	}
	// Convert the map back to a slice.
	var allAgentBinariesSlice []agentbinary.Metadata
	for _, ab := range allAgentBinaries {
		allAgentBinariesSlice = append(allAgentBinariesSlice, ab)
	}
	return allAgentBinariesSlice, nil
}
