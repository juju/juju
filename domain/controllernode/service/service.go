// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

// State describes retrieval and persistence
// methods for controller node concerns.
type State interface {
	CurateNodes(context.Context, []string, []string) error
	UpdateDqliteNode(context.Context, string, uint64, string) error
	SelectModelUUID(context.Context, model.UUID) (model.UUID, error)
}

// Service provides the API for working with controller nodes.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{st}
}

// CurateNodes modifies the known control plane by adding and removing
// controller node records according to the input slices.
func (s *Service) CurateNodes(ctx context.Context, toAdd, toRemove []string) error {
	err := s.st.CurateNodes(ctx, toAdd, toRemove)
	return errors.Annotatef(err, "curating controller codes; adding %v, removing %v", toAdd, toRemove)
}

// UpdateDqliteNode sets the Dqlite node ID and bind address for the input
// controller ID.
func (s *Service) UpdateDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	err := s.st.UpdateDqliteNode(ctx, controllerID, nodeID, addr)
	return errors.Annotatef(err, "updating Dqlite node details for %q", controllerID)
}

// IsModelKnownToController returns true if the input
// model UUID is one managed by this controller.
func (s *Service) IsModelKnownToController(ctx context.Context, modelUUID model.UUID) (bool, error) {
	if err := modelUUID.Validate(); err != nil {
		return false, fmt.Errorf("validating is known model uuid %q: %w", modelUUID, err)
	}

	uuid, err := s.st.SelectModelUUID(ctx, modelUUID)
	if err != nil {
		if !errors.Is(err, modelerrors.NotFound) {
			return false, errors.Annotatef(err, "determining model existence")
		}
	}

	return uuid == modelUUID, nil
}
