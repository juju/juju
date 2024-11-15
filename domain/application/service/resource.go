// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resources"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

// ResourceState describes retrieval and persistence methods for resources.
type ResourceState interface {
	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (resources.ID, error)

	// ListResources returns the list of resources for the given application.
	ListResources(ctx context.Context, applicationID application.ID) (resource.ApplicationResources, error)

	// GetResource returns the identified resource.
	GetResource(ctx context.Context, resourceID resources.ID) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(ctx context.Context, config resource.SetResourceArgs) (resource.Resource, error)

	// SetUnitResource sets the resource metadata for a specific unit.
	SetUnitResource(ctx context.Context, config resource.SetUnitResourceArgs) (resource.SetUnitResourceResult, error)

	// OpenApplicationResource returns the metadata for an application's resource.
	OpenApplicationResource(ctx context.Context, resourceID resources.ID) (resource.Resource, error)

	// OpenUnitResource returns the metadata for a resource a. A unit resource is
	// created to track the given unit and which resource its using.
	OpenUnitResource(ctx context.Context, resourceID resources.ID, unitID coreunit.UUID) (resource.Resource, error)

	// SetRepositoryResources sets the "polled" resources for the
	// application to the provided values. The current data for this
	// application/resource combination will be overwritten.
	SetRepositoryResources(ctx context.Context, config resource.SetRepositoryResourcesArgs) error
}

type ResourceStoreGetter interface {
	GetResourceStore(context.Context, charmresource.Type) (resource.ResourceStore, error)
}

// ResourceService provides the API for working with resources.
type ResourceService struct {
	st                  ResourceState
	resourceStoreGetter ResourceStoreGetter
	logger              logger.Logger
}

// NewResourceService returns a new service reference wrapping the input state.
func NewResourceService(st ResourceState, resourceStoreGetter ResourceStoreGetter, logger logger.Logger) *ResourceService {
	return &ResourceService{
		st:                  st,
		resourceStoreGetter: resourceStoreGetter,
		logger:              logger.Child("resource"),
	}
}

// GetApplicationResourceID returns the ID of the application resource specified by
// natural key of application and resource name.
//
// The following error types can be expected to be returned:
//   - application.ResourceNameNotValid if no resource name is provided in
//     the args.
//   - errors.NotValid is returned if the application ID is not valid.
//   - application.ResourceNotFound if no resource with name exists for
//     given application.
func (s *ResourceService) GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (resources.ID, error) {
	if err := args.ApplicationID.Validate(); err != nil {
		return "", fmt.Errorf("application id: %w", err)
	}
	if args.Name == "" {
		return "", applicationerrors.ResourceNameNotValid
	}
	return s.st.GetApplicationResourceID(ctx, args)
}

// ListResources returns the resource data for the given application including
// application, unit and repository resource data. Unit data is only included
// for machine units. Repository resource data is included if it exists.
//
// The following error types can be expected to be returned:
//   - errors.NotValid is returned if the application ID is not valid.
//   - application.ApplicationDyingOrDead for dead or dying applications.
//   - application.ApplicationNotFound when the specified application does
//     not exist.
//
// No error is returned if the provided application has no resources.
func (s *ResourceService) ListResources(ctx context.Context, applicationID application.ID) (resource.ApplicationResources, error) {
	if err := applicationID.Validate(); err != nil {
		return resource.ApplicationResources{}, fmt.Errorf("application id: %w", err)
	}
	return s.st.ListResources(ctx, applicationID)
}

// GetResource returns the identified application resource.
//
// The following error types can be expected to be returned:
//   - errors.NotValid is returned if the application ID is not valid.
//   - application.ApplicationDyingOrDead for dead or dying applications.
//   - application.ApplicationNotFound if the specified application does
//     not exist.
func (s *ResourceService) GetResource(ctx context.Context, resourceID resources.ID) (resource.Resource, error) {
	if err := resourceID.Validate(); err != nil {
		return resource.Resource{}, fmt.Errorf("application id: %w", err)
	}
	return s.st.GetResource(ctx, resourceID)
}

// SetResource adds the application resource to blob storage and updates the metadata.
//
// The following error types can be expected to be returned:
//   - errors.NotValid is returned if the application ID is not valid.
//   - errors.NotValid is returned if the resource is not valid.
//   - errors.NotValid is returned if the SuppliedByType is unknown while
//     SuppliedBy has a value.
//   - application.ApplicationNotFound if the specified application does
//     not exist.
func (s *ResourceService) SetResource(ctx context.Context, args resource.SetResourceArgs) (resource.Resource, error) {
	if err := args.ApplicationID.Validate(); err != nil {
		return resource.Resource{}, fmt.Errorf("application id: %w", err)
	}
	if args.SuppliedBy != "" && args.SuppliedByType == resource.Unknown {
		return resource.Resource{}, fmt.Errorf("%w SuppliedByType cannot be unknown if SuppliedBy set", errors.NotValid)
	}
	if err := args.Resource.Validate(); err != nil {
		return resource.Resource{}, fmt.Errorf("resource: %w", err)
	}
	return s.st.SetResource(ctx, args)
}

// SetUnitResource sets the resource metadata for a specific unit.
//
// The following error types can be expected to be returned:
//   - errors.NotValid is returned if the unit UUID is not valid.
//   - errors.NotValid is returned if the resource is not valid.
//   - errors.NotValid is returned if the SuppliedByType is unknown while
//     SuppliedBy has a value.
//   - application.ApplicationNotFound if the specified application does
//     not exist.
func (s *ResourceService) SetUnitResource(ctx context.Context, args resource.SetUnitResourceArgs) (resource.SetUnitResourceResult, error) {
	if err := args.UnitID.Validate(); err != nil {
		return resource.SetUnitResourceResult{}, fmt.Errorf("unit id: %w", err)
	}
	if args.SuppliedBy != "" && args.SuppliedByType == resource.Unknown {
		return resource.SetUnitResourceResult{}, fmt.Errorf("%w SuppliedByType cannot be unknown if SuppliedBy set", errors.NotValid)
	}
	if err := args.Resource.Validate(); err != nil {
		return resource.SetUnitResourceResult{}, fmt.Errorf("resource: %w", err)
	}
	return s.st.SetUnitResource(ctx, args)
}

// OpenApplicationResource returns the details of and a reader for the resource.
//
// The following error types can be expected to be returned:
//   - errors.NotValid is returned if the resource ID is not valid.
//   - application.ResourceNotFound if the specified resource does
//     not exist.
func (s *ResourceService) OpenApplicationResource(ctx context.Context, resourceID resources.ID) (resource.Resource, io.ReadCloser, error) {
	if err := resourceID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("resource id: %w", err)
	}
	res, err := s.st.OpenApplicationResource(ctx, resourceID)
	return res, &noopReadCloser{}, err
}

// OpenUnitResource returns metadata about the resource and a reader for
// the resource. The resource is associated with the unit once the reader is
// completely exhausted. Read progress is stored until the reader is completely
// exhausted. Typically used for File resources.
//
// The following error types can be returned:
//   - errors.NotValid is returned if the resource ID is not valid.
//   - errors.NotValid is returned if the unit UUID is not valid.
//   - application.ResourceNotFound if the specified resource does
//     not exist.
//   - application.UnitNotFound if the specified unit does
//     not exist.
func (s *ResourceService) OpenUnitResource(ctx context.Context, resourceID resources.ID, unitID coreunit.UUID) (resource.Resource, io.ReadCloser, error) {
	if err := unitID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("unit id: %w", err)
	}
	if err := resourceID.Validate(); err != nil {
		return resource.Resource{}, nil, fmt.Errorf("resource id: %w", err)
	}
	res, err := s.st.OpenUnitResource(ctx, resourceID, unitID)
	return res, &noopReadCloser{}, err
}

// SetRepositoryResources sets the "polled" resources for the application to
// the provided values. These are resources collected from the repository for
// the application.
//
// The following error types can be expected to be returned:
//   - errors.NotValid is returned if the Application ID is not valid.
//   - errors.NotValid is returned if LastPolled is zero.
//   - errors.NotValid is returned if the length of Info is zero.
//   - application.ApplicationNotFound if the specified application does
//     not exist.
func (s *ResourceService) SetRepositoryResources(ctx context.Context, args resource.SetRepositoryResourcesArgs) error {
	if err := args.ApplicationID.Validate(); err != nil {
		return fmt.Errorf("application id: %w", err)
	}
	if len(args.Info) == 0 {
		return fmt.Errorf("empty Info %w", errors.NotValid)
	}
	for _, info := range args.Info {
		if err := info.Validate(); err != nil {
			return fmt.Errorf("resource: %w", err)
		}
	}
	if args.LastPolled.IsZero() {
		return fmt.Errorf("zero LastPolled %w", errors.NotValid)
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
