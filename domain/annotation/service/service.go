// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/annotations"
)

// State describes retrieval and persistence methods for annotations.
type State interface {
	// GetAnnotations retrieves all the annotations associated with a given ID.
	// If no annotations are found, an empty map is returned.
	GetAnnotations(ctx context.Context, ID annotations.ID) (map[string]string, error)

	// SetAnnotations associates key/value annotation pairs with a given ID.
	// If annotation already exists for the given ID, then it will be updated with
	// the given value.
	SetAnnotations(ctx context.Context, ID annotations.ID, annotations map[string]string) error
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

// GetAnnotations retrieves all the annotations associated with a given ID.
// If no annotations are found, an empty map is returned.
func (s *Service) GetAnnotations(ctx context.Context, ID annotations.ID) (map[string]string, error) {
	annotations, err := s.st.GetAnnotations(ctx, ID)
	return annotations, errors.Trace(err)
}

// SetAnnotations associates key/value annotation pairs with a given ID.
// If annotation already exists for the given ID, then it will be updated with
// the given value.
func (s *Service) SetAnnotations(ctx context.Context, ID annotations.ID, annotations map[string]string) error {
	err := s.st.SetAnnotations(ctx, ID, annotations)
	return errors.Annotatef(err, "updating annotations for ID: %q", ID.Name)
}
