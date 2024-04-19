// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
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
	return errors.Annotatef(err, "curating controller codes; adding %v, removing %v", toAdd, toRemove)
}

// UpdateDqliteNode sets the Dqlite node ID and bind address for the input
// controller ID.
func (s *Service) UpdateDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	err := s.st.UpdateDqliteNode(ctx, controllerID, nodeID, addr)
	return errors.Annotatef(err, "updating Dqlite node details for %q", controllerID)
}

// IsKnownDatabaseNamespace reports if the namespace is known to the controller.
// If the namespace is not valid an error satisfying [errors.NotValid] is
// returned.
func (s *Service) IsKnownDatabaseNamespace(ctx context.Context, namespace string) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("namespace %q is %w, cannot be empty", namespace, errors.NotValid)
	}

	ns, err := s.st.SelectDatabaseNamespace(ctx, namespace)
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			return false, errors.Annotatef(err, "determining namespace existence")
		}
	}

	return ns == namespace, nil
}
