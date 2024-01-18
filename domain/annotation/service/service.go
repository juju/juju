// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/state"
)

// State describes retrieval and persistence methods for annotations.
type State interface {
	// GetAnnotations retrieves all the annotations associated with a given entity.
	// If no annotations are found, an empty map is returned.
	GetAnnotations(ctx context.Context, entity state.GlobalEntity) (map[string]string, error)

	// SetAnnotations adds key/value pairs to the annotations in the corresponding
	// table for a given entity. If a given annotation already exists for the given entity
	// in the database, then it will be updated.
	SetAnnotations(ctx context.Context, entity state.GlobalEntity, annotations map[string]string) error
}

// Service provides the API for working with annotations.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the given annotations state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// GetAnnotations retrieves all annotations associated with a given entity.
// If no annotations are found, an empty map is returned.
func (s *Service) GetAnnotations(ctx context.Context, entity state.GlobalEntity) (map[string]string, error) {
	annotations, err := s.st.GetAnnotations(ctx, entity)
	return annotations, errors.Trace(err)
}

// SetAnnotations adds key/value pairs to the annotations in the corresponding
// table for a given entity. If a given annotation already exists for the given entity
// in the database, then it will be updated.
func (s *Service) SetAnnotations(ctx context.Context, entity state.GlobalEntity, annotations map[string]string) error {
	err := s.st.SetAnnotations(ctx, entity, annotations)
	return errors.Annotatef(err, "updating annotations for entity")
}
