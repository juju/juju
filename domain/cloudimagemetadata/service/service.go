// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/cloudimagemetadata"
	cloudimageerrors "github.com/juju/juju/domain/cloudimagemetadata/errors"
	"github.com/juju/juju/internal/errors"
)

// State is an interface of the persistence layer for managing cloud image metadata
type State interface {
	// SaveMetadata saves a list of cloud image metadata to the state.
	SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error

	// DeleteMetadataWithImageID removes the metadata associated with the given imageID from the state.
	DeleteMetadataWithImageID(ctx context.Context, imageID string) error

	// FindMetadata retrieves a list of cloud image metadata based on the provided criteria.
	// It returns the matched metadata or a [github.com/juju/juju/domain/cloudimagemetadata/errors.NotFound] error
	// if there are not one.
	FindMetadata(ctx context.Context, criteria cloudimagemetadata.MetadataFilter) ([]cloudimagemetadata.Metadata, error)

	// AllCloudImageMetadata retrieves all cloud image metadata from the state and returns them as a list.
	AllCloudImageMetadata(ctx context.Context) ([]cloudimagemetadata.Metadata, error)

	// SupportedArchitectures returns a set of strings representing the architectures supported by the cloud images.
	SupportedArchitectures(ctx context.Context) set.Strings
}

// Service provides the API for working with cloud image metadata
type Service struct {
	st State
}

// NewService creates a new instance of Service using the provided State.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// SaveMetadata saves the provided cloud image metadata if non-empty and valid.
// It returns a [errors.NotValid] if at least one of the inputs are invalid.
func (s Service) SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if len(metadata) == 0 {
		return nil
	}

	if err := s.validateAllMetadata(ctx, metadata); err != nil {
		return err
	}

	return s.st.SaveMetadata(ctx, metadata)
}

// DeleteMetadataWithImageID removes all the metadata associated with the given imageID from the state.
// It returns a [errors.EmptyImageID] if the provided imageID is empty.
func (s Service) DeleteMetadataWithImageID(ctx context.Context, imageID string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if imageID == "" {
		return cloudimageerrors.EmptyImageID
	}

	return s.st.DeleteMetadataWithImageID(ctx, imageID)
}

// FindMetadata retrieves a map of image metadata grouped by the source based on the provided filter criteria.
func (s Service) FindMetadata(ctx context.Context, criteria cloudimagemetadata.MetadataFilter) (_ map[string][]cloudimagemetadata.Metadata, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	metadata, err := s.st.FindMetadata(ctx, criteria)
	if err != nil {
		return nil, err
	}
	metadataBySource := make(map[string][]cloudimagemetadata.Metadata)
	for _, v := range metadata {
		metadataBySource[v.Source] = append(metadataBySource[v.Source], v)
	}
	return metadataBySource, nil
}

// AllCloudImageMetadata retrieves all cloud image metadata from the state and returns them as a list.
func (s Service) AllCloudImageMetadata(ctx context.Context) (_ []cloudimagemetadata.Metadata, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()
	return s.st.AllCloudImageMetadata(ctx)
}

// validateAllMetadata validates a list of cloud image metadata.
// It returns an error if any of the metadata entries are invalid.
func (s Service) validateAllMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error {
	var errs []error
	for _, m := range metadata {
		errs = append(errs, s.validateMetadata(ctx, m))
	}
	return errors.Join(errs...)
}

// validateMetadata validates a single cloud image metadata entry.
func (s Service) validateMetadata(ctx context.Context, m cloudimagemetadata.Metadata) error {
	if m.ImageID == "" {
		return errors.Errorf("%w: %w", cloudimageerrors.EmptyImageID, cloudimageerrors.NotValid)
	}

	var missing []string
	if m.Version == "" {
		missing = append(missing, "version")
	}
	if m.Stream == "" {
		missing = append(missing, "stream")
	}
	if m.Source == "" {
		missing = append(missing, "source")
	}
	if m.Arch == "" {
		missing = append(missing, "arch")
	}
	if m.Region == "" {
		missing = append(missing, "region")
	}

	if len(missing) > 0 {
		return cloudimageerrors.NotValidMissingFields(m.ImageID, missing)
	}

	supportedArchitectures := s.st.SupportedArchitectures(ctx)
	if !supportedArchitectures.Contains(m.Arch) {
		return errors.Errorf("unsupported architecture %s (should be any of %s): %w", m.Arch, supportedArchitectures.Values(), cloudimageerrors.NotValid)
	}

	return nil
}
