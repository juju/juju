// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/annotation"
	annotationerrors "github.com/juju/juju/domain/annotation/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for annotations.
type State interface {
	// GetAnnotations retrieves all the annotations associated with a given ID.
	// If no annotations are found, an empty map is returned.
	GetAnnotations(ctx context.Context, ID annotations.ID) (map[string]string, error)

	// GetCharmAnnotations retrieves all the annotations associated with a given
	// ID. If no annotations are found, an empty map is returned.
	GetCharmAnnotations(ctx context.Context, ID annotation.GetCharmArgs) (map[string]string, error)

	// SetAnnotations associates key/value annotation pairs with a given ID.
	// If an annotation already exists for the given ID, then it will be updated
	// with the given value.
	// Annotation keys not included will be left unchaged.
	// Annotation keys in the deletions slice will be deleted.
	SetAnnotations(ctx context.Context, ID annotations.ID, upserts map[string]string, deletions []string) error

	// SetCharmAnnotations associates key/value annotation pairs with a given ID.
	// If an annotation already exists for the given ID, then it will be updated
	// with the given value.
	// First all annotations are deleted, then the given
	// pairs are inserted, so unsetting an annotation is implicit.
	SetCharmAnnotations(ctx context.Context, ID annotation.GetCharmArgs, annotations map[string]string) error
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

// GetAnnotations retrieves all the annotations associated with a given ID. If
// no annotations are found, an empty map is returned.
func (s *Service) GetAnnotations(ctx context.Context, id annotations.ID) (map[string]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	annotations, err := s.st.GetAnnotations(ctx, id)
	return annotations, errors.Capture(err)
}

// GetCharmAnnotations retrieves all the annotations associated with a given
// charm argument. If no annotations are found, an empty map is returned.
func (s *Service) GetCharmAnnotations(ctx context.Context, id annotation.GetCharmArgs) (map[string]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	annotations, err := s.st.GetCharmAnnotations(ctx, id)
	return annotations, errors.Capture(err)
}

// SetAnnotations associates key/value annotation pairs with a given ID. If
// an annotation already exists for the given ID, then it will be updated with
// the given value.
// Annotation keys not included will be left unchaged.
// Anotation keys with an empty string value will be removed.
func (s *Service) SetAnnotations(ctx context.Context, id annotations.ID, annotations map[string]string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return errors.Capture(err)
	}

	if err := s.validateAnnotations(annotations); err != nil {
		return errors.Capture(err)
	}

	// To maintain API compatibility with the 3.x series, treat empty string values as deletions
	upserts := make(map[string]string)
	deletions := make([]string, 0)
	for key, value := range annotations {
		if value != "" {
			upserts[key] = value
		} else {
			deletions = append(deletions, key)
		}
	}

	if err := s.st.SetAnnotations(ctx, id, upserts, deletions); err != nil {
		return errors.Errorf("updating annotations for %q: %w", id.Name, err)
	}
	return nil
}

// SetCharmAnnotations associates key/value annotation pairs with a given charm
// argument. If an annotation already exists for the given ID, then it will be
// updated with the given value.
func (s *Service) SetCharmAnnotations(ctx context.Context, id annotation.GetCharmArgs, annotations map[string]string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return errors.Capture(err)
	}

	if err := s.validateAnnotations(annotations); err != nil {
		return errors.Capture(err)
	}

	if err := s.st.SetCharmAnnotations(ctx, id, annotations); err != nil {
		return errors.Errorf("updating annotations for %q: %w", id.Name, err)
	}
	return nil
}

func (s *Service) validateAnnotations(annotations map[string]string) error {
	for key := range annotations {
		if strings.Contains(key, ".") {
			return errors.Errorf("key %q contains period: %w", key, annotationerrors.InvalidKey)
		}

		k := strings.TrimSpace(key)
		if k == "" {
			return errors.Errorf("key is empty string: %w", annotationerrors.InvalidKey)
		}
	}

	return nil
}
