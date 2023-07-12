// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
)

// State describes retrieval and persistence
// methods for controller node concerns.
type State interface {
	CurateNodes(context.Context, []string, []string) error
	UpdateBootstrapNodeBindAddress(context.Context, string) error
	SelectModelUUID(context.Context, string) (string, error)
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

// UpdateBootstrapNodeBindAddress sets the input address as the Dqlite
// bind address of the original bootstrapped controller node.
func (s *Service) UpdateBootstrapNodeBindAddress(ctx context.Context, addr string) error {
	err := s.st.UpdateBootstrapNodeBindAddress(ctx, addr)
	return errors.Annotatef(err, "updating bootstrap node bind address to %q", addr)
}

func (s *Service) IsModelKnownToController(ctx context.Context, modelUUID string) (bool, error) {
	uuid, err := s.st.SelectModelUUID(ctx, modelUUID)
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			return false, errors.Annotatef(err, "determining model existence")
		}
	}

	return uuid == modelUUID, nil
}
