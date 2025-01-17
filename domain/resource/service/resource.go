// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	coreunit "github.com/juju/juju/core/unit"
	containerimageresourcestoreerrors "github.com/juju/juju/domain/containerimageresourcestore/errors"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for resource.
type State interface {
	// DeleteApplicationResources removes all associated resources of a given
	// application identified by applicationID.
	DeleteApplicationResources(ctx context.Context, applicationID coreapplication.ID) error

	// DeleteUnitResources deletes the association of resources with a specific
	// unit.
	DeleteUnitResources(ctx context.Context, uuid coreunit.UUID) error

	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (coreresource.UUID, error)

	// ListResources returns the list of resource for the given application.
	ListResources(ctx context.Context, applicationID coreapplication.ID) (resource.ApplicationResources, error)

	// GetResource returns the identified resource.
	GetResource(ctx context.Context, resourceUUID coreresource.UUID) (resource.Resource, error)

	// GetResourceType finds the type of the given resource from the resource table.
	//
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the resource UUID cannot be
	//     found.
	GetResourceType(ctx context.Context, resourceUUID coreresource.UUID) (charmresource.Type, error)

	// RecordStoredResource records a stored resource along with who retrieved it.
	//
	// The following error types can be expected to be returned:
	// - [resourceerrors.StoredResourceNotFound] if the stored resource at the
	//   storageID cannot be found.
	RecordStoredResource(ctx context.Context, args resource.RecordStoredResourceArgs) (droppedHash string, err error)

	// SetUnitResource sets the resource metadata for a specific unit.
	//
	// The following error types can be expected to be returned:
	//  - [resourceerrors.UnitNotFound] if the unit id doesn't belong to an
	//    existing unit.
	//  - [resourceerrors.ResourceNotFound] if the resource id doesn't belong
	//    to an existing resource.
	SetUnitResource(ctx context.Context, resourceUUID coreresource.UUID, unitUUID coreunit.UUID) error

	// SetApplicationResource marks an existing resource as in use by a CAAS
	// application.
	//
	// The following error types can be expected to be returned:
	//  - [resourceerrors.ResourceNotFound] if the resource id doesn't belong
	//    to an existing resource.
	SetApplicationResource(ctx context.Context, resourceUUID coreresource.UUID) error

	// SetRepositoryResources sets the "polled" resource for the
	// application to the provided values. The current data for this
	// application/resource combination will be overwritten.
	SetRepositoryResources(ctx context.Context, config resource.SetRepositoryResourcesArgs) error
}

type ResourceStoreGetter interface {
	// GetResourceStore returns the appropriate ResourceStore for the
	// given resource type.
	GetResourceStore(context.Context, charmresource.Type) (coreresourcestore.ResourceStore, error)
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

// DeleteApplicationResources removes the resources of a specified application.
// It should be called after all resources have been unlinked from potential
// units by DeleteUnitResources and their associated data removed from store.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationIDNotValid] is returned if the application
//     ID is not valid.
//   - [resourceerrors.CleanUpStateNotValid] is returned is there is
//     remaining units or stored resources which are still associated with
//     application resources.
func (s *Service) DeleteApplicationResources(
	ctx context.Context,
	applicationID coreapplication.ID,
) error {
	if err := applicationID.Validate(); err != nil {
		return resourceerrors.ApplicationIDNotValid
	}
	return s.st.DeleteApplicationResources(ctx, applicationID)
}

// DeleteUnitResources unlinks the resources associated to a unit by its UUID.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.UnitUUIDNotValid] is returned if the unit ID is not
//     valid.
func (s *Service) DeleteUnitResources(
	ctx context.Context,
	uuid coreunit.UUID,
) error {
	if err := uuid.Validate(); err != nil {
		return resourceerrors.UnitUUIDNotValid
	}
	return s.st.DeleteUnitResources(ctx, uuid)
}

// GetApplicationResourceID returns the ID of the application resource specified
// by natural key of application and resource name.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNameNotValid] if no resource name is provided
//     in the args.
//   - [coreerrors.NotValid] is returned if the application ID is not valid.
//   - [resourceerrors.ResourceNotFound] if no resource with name exists for
//     given application.
func (s *Service) GetApplicationResourceID(
	ctx context.Context,
	args resource.GetApplicationResourceIDArgs,
) (coreresource.UUID, error) {
	if err := args.ApplicationID.Validate(); err != nil {
		return "", errors.Errorf("application id: %w", err)
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
//   - [resourceerrors.ApplicationDyingOrDead] for dead or dying
//     applications.
//   - [resourceerrors.ApplicationNotFound] when the specified application
//     does not exist.
//
// No error is returned if the provided application has no resource.
func (s *Service) ListResources(
	ctx context.Context,
	applicationID coreapplication.ID,
) (resource.ApplicationResources, error) {
	if err := applicationID.Validate(); err != nil {
		return resource.ApplicationResources{}, errors.Errorf("application id: %w", err)
	}
	return s.st.ListResources(ctx, applicationID)
}

// GetResource returns the identified application resource.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) GetResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, error) {
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, errors.Errorf("application id: %w", err)
	}
	return s.st.GetResource(ctx, resourceUUID)
}

// StoreResource adds the application resource to blob storage and updates the
// metadata. It also sets the retrival information for the resource.
//
// The Size and Fingerprint should be validated against the resource blob before
// the resource is passed in.
//
// If storing a blob for a resource that already has a blob stored, the old blob
// will be replaced and removed from the store.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if the resource UUID cannot be
//     found.
//   - [resourceerrors.RetrievedByTypeNotValid] if the retrieved by type is
//     invalid.
func (s *Service) StoreResource(
	ctx context.Context,
	args resource.StoreResourceArgs,
) error {
	return s.storeResource(ctx, args, false)
}

// StoreResourceAndIncrementCharmModifiedVersion adds the application resource
// to blob storage and updates the metadata. It sets the retrival information
// for the resource and also increments the charm modified version for the
// resources' application.
//
// The Size and Fingerprint should be validated against the resource blob before
// the resource is passed in.
//
// If storing a blob for a resource that already has a blob stored, the old blob
// will be replaced and removed from the store.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if the resource UUID cannot be
//     found.
//   - [resourceerrors.RetrievedByTypeNotValid] if the retrieved by type is
//     invalid.
func (s *Service) StoreResourceAndIncrementCharmModifiedVersion(
	ctx context.Context,
	args resource.StoreResourceArgs,
) error {
	return s.storeResource(ctx, args, true)
}

func (s *Service) storeResource(
	ctx context.Context,
	args resource.StoreResourceArgs,
	incrementCharmModifiedVersion bool,
) (err error) {
	if err = args.ResourceUUID.Validate(); err != nil {
		return errors.Errorf("resource uuid: %w", err)
	}

	if args.Reader == nil {
		return errors.Errorf("cannot have nil reader")
	}
	if args.Size < 0 {
		return errors.Errorf("invalid size: %d", args.Size)
	}
	if args.Fingerprint.IsZero() {
		return errors.Errorf("invalid fingerprint")
	}

	if args.RetrievedBy != "" && args.RetrievedByType == resource.Unknown {
		return resourceerrors.RetrievedByTypeNotValid
	}

	res, err := s.st.GetResource(ctx, args.ResourceUUID)
	if err != nil {
		return errors.Errorf("getting resource: %w", err)
	}

	if args.Fingerprint.String() == res.Fingerprint.String() {
		// This resource blob has already been stored, no need to store it
		// again.
		return nil
	}

	store, err := s.resourceStoreGetter.GetResourceStore(ctx, res.Type)
	if err != nil {
		return errors.Errorf("getting resource store for %s: %w", res.Type.String(), err)
	}

	path := blobPath(args.ResourceUUID, args.Fingerprint.String())
	storageUUID, err := store.Put(
		ctx,
		path,
		args.Reader,
		args.Size,
		coreresourcestore.NewFingerprint(args.Fingerprint.Fingerprint),
	)
	if err != nil {
		return errors.Errorf("putting resource %q in store: %w", res.Name, err)
	}
	defer func() {
		// If any subsequent operation fails, remove the resource blob.
		if err != nil {
			_ = store.Remove(ctx, path)
		}
	}()

	droppedHash, err := s.st.RecordStoredResource(
		ctx,
		resource.RecordStoredResourceArgs{
			ResourceUUID:                  args.ResourceUUID,
			StorageID:                     storageUUID,
			RetrievedBy:                   args.RetrievedBy,
			RetrievedByType:               args.RetrievedByType,
			ResourceType:                  res.Type,
			IncrementCharmModifiedVersion: incrementCharmModifiedVersion,
			Size:                          args.Size,
			SHA384:                        args.Fingerprint.String(),
		},
	)
	if err != nil {
		return errors.Errorf("recording stored resource %q: %w", res.Name, err)
	}

	// If the resource was updated and an old resource blob was dropped, remove
	// the old blob from the store.
	if droppedHash != "" {
		err = store.Remove(ctx, blobPath(args.ResourceUUID, droppedHash))
		if err != nil {
			s.logger.Errorf("failed to remove resource with ID %s from the store", droppedHash)
		}
	}
	return err
}

// OpenResource returns the details of and a reader for the resource.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if the specified resource does not
//     exist.
//   - [resourceerrors.StoredResourceNotFound] if the specified resource is not
//     in the resource store.
func (s *Service) OpenResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, io.ReadCloser, error) {
	if err := resourceUUID.Validate(); err != nil {
		return resource.Resource{}, nil, errors.Errorf("resource id: %w", err)
	}

	res, err := s.st.GetResource(ctx, resourceUUID)
	if err != nil {
		return resource.Resource{}, nil, err
	}

	store, err := s.resourceStoreGetter.GetResourceStore(ctx, res.Type)
	if err != nil {
		return resource.Resource{}, nil, errors.Errorf("getting resource store for %s: %w", res.Type.String(), err)
	}

	// TODO(aflynn): ideally this would be finding the resource via the
	// resources storageID, however the object store does not currently have a
	// method for this.
	path := resourceUUID.String() + res.Fingerprint.String()
	reader, _, err := store.Get(ctx, path)
	if errors.Is(err, objectstoreerrors.ErrNotFound) ||
		errors.Is(err, containerimageresourcestoreerrors.ContainerImageMetadataNotFound) {
		return resource.Resource{}, nil, resourceerrors.StoredResourceNotFound
	} else if err != nil {
		return resource.Resource{}, nil, errors.Errorf("getting resource from store: %w", err)
	}

	return res, reader, nil
}

// SetUnitResource sets the unit as using the resource.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.UnitNotFound] if the unit id doesn't belong to an
//     existing unit.
//   - [resourceerrors.ResourceNotFound] if the resource id doesn't belong
//     to an existing resource.
func (s *Service) SetUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitUUID coreunit.UUID,
) error {
	if err := resourceUUID.Validate(); err != nil {
		return errors.Errorf("resource id: %w", err)
	}

	if err := unitUUID.Validate(); err != nil {
		return errors.Errorf("unit uuid: %w", err)
	}

	err := s.st.SetUnitResource(ctx, resourceUUID, unitUUID)
	if err != nil {
		return errors.Errorf("recording resource for unit: %w", err)
	}
	return nil
}

// SetApplicationResource marks an existing resource as in use by a CAAS
// application.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if the resource UUID cannot be
//     found.
func (s *Service) SetApplicationResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) error {
	if err := resourceUUID.Validate(); err != nil {
		return errors.Errorf("resource id: %w", err)
	}

	err := s.st.SetApplicationResource(ctx, resourceUUID)
	if err != nil {
		return errors.Errorf("recording resource for application: %w", err)
	}
	return nil
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
		return errors.Errorf("application id: %w", err)
	}
	if len(args.Info) == 0 {
		return errors.Errorf("empty Info: %w", resourceerrors.ArgumentNotValid)
	}
	for _, info := range args.Info {
		if err := info.Validate(); err != nil {
			return errors.Errorf("resource: %w", err)
		}
	}
	if args.LastPolled.IsZero() {
		return errors.Errorf("zero LastPolled: %w", resourceerrors.ArgumentNotValid)
	}
	return s.st.SetRepositoryResources(ctx, args)
}

// Store the resource with a path made up of the UUID and the resource
// hash, this ensures that different resource blobs are stored in
// different locations.
func blobPath(uuid coreresource.UUID, hash string) string {
	return uuid.String() + hash
}
