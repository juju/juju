// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"io"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/store"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// ResourceState describes retrieval and persistence methods for resource.
type State interface {
	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (coreresource.UUID, error)

	// ListResources returns the list of resource for the given application.
	ListResources(ctx context.Context, applicationID coreapplication.ID) (resource.ApplicationResources, error)

	// GetResource returns the identified resource.
	GetResource(ctx context.Context, resourceUUID coreresource.UUID) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(ctx context.Context, config resource.SetResourceArgs) (resource.Resource, error)

	// SetUnitResource sets the resource metadata for a specific unit.
	SetUnitResource(ctx context.Context, config resource.SetUnitResourceArgs) (resource.SetUnitResourceResult, error)

	// OpenApplicationResource returns the metadata for an application's resource.
	OpenApplicationResource(ctx context.Context, resourceUUID coreresource.UUID) (resource.Resource, error)

	// OpenUnitResource returns the metadata for a resource a. A unit resource is
	// created to track the given unit and which resource its using.
	OpenUnitResource(ctx context.Context, resourceUUID coreresource.UUID, unitID coreunit.UUID) (resource.Resource, error)

	// SetRepositoryResources sets the "polled" resource for the
	// application to the provided values. The current data for this
	// application/resource combination will be overwritten.
	SetRepositoryResources(ctx context.Context, config resource.SetRepositoryResourcesArgs) error
}

type ResourceStoreGetter interface {
	// GetResourceStore returns the appropriate ResourceStore for the
	// given resource type.
	GetResourceStore(context.Context, charmresource.Type) (store.ResourceStore, error)
}

// Service provides the API for working with resources.
type Service struct {
	st     State
	logger logger.Logger

	resourceStoreGetter ResourceStoreGetter
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	resourceStoreGetter ResourceStoreGetter,
	logger logger.Logger,
) *Service {
	return &Service{
		st:                  st,
		resourceStoreGetter: resourceStoreGetter,
		logger:              logger,
	}
}

// GetApplicationResourceID returns the ID of the application resource specified by
// natural key of application and resource name.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNameNotValid] if no resource name is provided in
//     the args.
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [resourceerrors.ResourceNotFound] if no resource with name exists for
//     given application.
func (s *Service) GetApplicationResourceID(
	ctx context.Context,
	args resource.GetApplicationResourceIDArgs,
) (coreresource.UUID, error) {
	if err := args.ApplicationID.Validate(); err != nil {
		return "", fmt.Errorf("application id: %w", err)
	}
	if args.Name == "" {
		return "", resourceerrors.ResourceNameNotValid
	}
	return s.st.GetApplicationResourceID(ctx, args)
}

// ListResources returns the resource data for the given application including
// application, unit and repository resource data. Unit data is only included
// for machine units. Repository resource data is included if it exists.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [applicationerrors.ApplicationDyingOrDead] for dead or dying applications.
//   - [applicationerrors.ApplicationNotFound] when the specified application does
//     not exist.
//
// No error is returned if the provided application has no resource.
func (s *Service) ListResources(
	ctx context.Context,
	applicationID coreapplication.ID,
) (resource.ApplicationResources, error) {
	if err := applicationID.Validate(); err != nil {
		return resource.ApplicationResources{}, fmt.Errorf("application id: %w", err)
	}
	return s.st.ListResources(ctx, applicationID)
}

// GetResource returns the identified application resource.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [applicationerrors.ApplicationDyingOrDead] for dead or dying
//     applications.
//   - [applicationerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) GetResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, error) {
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, fmt.Errorf("application id: %w", err)
	}
	return s.st.GetResource(ctx, resourceUUID)
}

// SetResource adds the application resource to blob storage and updates the metadata.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [coreerrors.NotValid] is returned if the resource is not valid.
//   - [coreerrors.NotValid] is returned if the RetrievedByType is unknown while
//     RetrievedBy has a value.
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) SetResource(
	ctx context.Context,
	args resource.SetResourceArgs,
) (resource.Resource, error) {
	if err := args.ApplicationID.Validate(); err != nil {
		return resource.Resource{}, fmt.Errorf("application id: %w", err)
	}
	if args.SuppliedBy != "" && args.SuppliedByType == resource.Unknown {
		return resource.Resource{},
			fmt.Errorf("RetrievedByType cannot be unknown if RetrievedBy set: %w", resourceerrors.ArgumentNotValid)
	}
	if err := args.Resource.Validate(); err != nil {
		return resource.Resource{}, fmt.Errorf("resource: %w", err)
	}
	return s.st.SetResource(ctx, args)
}

// SetUnitResource sets the resource metadata for a specific unit.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the unit UUID is not valid.
//   - [coreerrors.NotValid] is returned if the resource UUID is not valid.
//   - [resourceerrors.ArgumentNotValid] is returned if the RetrievedByType is unknown while
//     RetrievedBy has a value.
//   - [resourceerrors.ResourceNotFound] if the specified resource doesn't exist
//   - [resourceerrors.UnitNotFound] if the specified unit doesn't exist
func (s *Service) SetUnitResource(
	ctx context.Context,
	args resource.SetUnitResourceArgs,
) (resource.SetUnitResourceResult, error) {
	if err := args.UnitUUID.Validate(); err != nil {
		return resource.SetUnitResourceResult{}, fmt.Errorf("unit id: %w", err)
	}
	if err := args.ResourceUUID.Validate(); err != nil {
		return resource.SetUnitResourceResult{}, fmt.Errorf("resource id: %w", err)
	}
	if args.RetrievedBy != "" && args.RetrievedByType == resource.Unknown {
		return resource.SetUnitResourceResult{},
			fmt.Errorf("RetrievedByType cannot be unknown if RetrievedBy set: %w", resourceerrors.ArgumentNotValid)
	}
	return s.st.SetUnitResource(ctx, args)
}

// OpenApplicationResource returns the details of and a reader for the resource.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the resource.UUID is not valid.
//   - [resourceerrors.ResourceNotFound] if the specified resource does
//     not exist.
func (s *Service) OpenApplicationResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, io.ReadCloser, error) {
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("resource id: %w", err)
	}
	res, err := s.st.OpenApplicationResource(ctx, resourceUUID)
	return res, &noopReadCloser{}, err
}

// OpenUnitResource returns metadata about the resource and a reader for
// the resource. The resource is associated with the unit once the reader is
// completely exhausted. Read progress is stored until the reader is completely
// exhausted. Typically used for File resource.
//
// The following error types can be returned:
//   - [coreerrors.NotValid] is returned if the resource.UUID is not valid.
//   - [coreerrors.NotValid] is returned if the unit UUID is not valid.
//   - [resourceerrors.ResourceNotFound] if the specified resource does
//     not exist.
//   - [resourceerrors.UnitNotFound] if the specified unit does
//     not exist.
func (s *Service) OpenUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitID coreunit.UUID,
) (resource.Resource, io.ReadCloser, error) {
	if err := unitID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("unit id: %w", err)
	}
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("resource id: %w", err)
	}
	res, err := s.st.OpenUnitResource(ctx, resourceUUID, unitID)
	return res, &noopReadCloser{}, err
}

// SetRepositoryResources sets the "polled" resource for the application to
// the provided values. These are resource collected from the repository for
// the application.
//
// The following error types can be expected to be returned:
//   - [coreerrors.NotValid] is returned if the Application ID is not valid.
//   - [resourceerrors.ArgumentNotValid] is returned if LastPolled is zero.
//   - [resourceerrors.ArgumentNotValid] is returned if the length of Info is zero.
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) SetRepositoryResources(
	ctx context.Context,
	args resource.SetRepositoryResourcesArgs,
) error {
	if err := args.ApplicationID.Validate(); err != nil {
		return fmt.Errorf("application id: %w", err)
	}
	if len(args.Info) == 0 {
		return fmt.Errorf("empty Info: %w", resourceerrors.ArgumentNotValid)
	}
	for _, info := range args.Info {
		if err := info.Validate(); err != nil {
			return fmt.Errorf("resource: %w", err)
		}
	}
	if args.LastPolled.IsZero() {
		return fmt.Errorf("zero LastPolled: %w", resourceerrors.ArgumentNotValid)
	}
	return s.st.SetRepositoryResources(ctx, args)
}

// TODO: remove me once OpenApplicationResource and OpenUnitResource implemented.
type noopReadCloser struct{}

func (noopReadCloser) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (noopReadCloser) Close() error {
	return nil
}
