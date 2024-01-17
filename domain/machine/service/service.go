// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
)

// State describes retrieval and persistence methods for machines.
type State interface {
	// UpsertMachine persists the input machine entity.
	UpsertMachine(context.Context, string) error

	// DeleteMachine deletes the input machine entity.
	DeleteMachine(context.Context, string) error
}

// Service provides the API for working with clouds.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// Save inserts or updates the specified machine.
func (s *Service) Save(ctx context.Context, machineId string) error {
	err := s.st.UpsertMachine(ctx, machineId)
	return errors.Annotatef(err, "saving machine %q", machineId)
}

// Delete deletes the specified machine.
func (s *Service) Delete(ctx context.Context, machineId string) error {
	err := s.st.DeleteMachine(ctx, machineId)
	return errors.Annotatef(err, "deleting machine %q", machineId)
}
