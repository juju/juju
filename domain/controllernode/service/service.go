// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence
// methods for controller node concerns.
type State interface {
	CurateNodes(context.Context, []string, []string) error
	UpdateDqliteNode(context.Context, string, uint64, string) error
	SelectDatabaseNamespace(context.Context, string) (string, error)
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
	if err != nil {
		return errors.Errorf("curating controller codes; adding %v, removing %v: %w", toAdd, toRemove, err)
	}
	return nil
}

// UpdateDqliteNode sets the Dqlite node ID and bind address for the input
// controller ID.
func (s *Service) UpdateDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	err := s.st.UpdateDqliteNode(ctx, controllerID, nodeID, addr)
	if err != nil {
		return errors.Errorf("updating Dqlite node details for %q: %w", controllerID, err)
	}
	return nil
}

// IsKnownDatabaseNamespace reports if the namespace is known to the controller.
// If the namespace is not valid an error satisfying [errors.NotValid] is
// returned.
func (s *Service) IsKnownDatabaseNamespace(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, errors.Errorf("namespace %q is %w, cannot be empty", namespace, coreerrors.NotValid)
	}

	ns, err := s.st.SelectDatabaseNamespace(ctx, namespace)
	if err != nil {
		if !errors.Is(err, controllernodeerrors.NotFound) {
			return false, errors.Errorf("determining namespace existence: %w", err)
		}
	}

	return ns == namespace, nil
}
