// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"regexp"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	coreunit "github.com/juju/juju/core/unit"
	containerimageresourcestoreerrors "github.com/juju/juju/domain/containerimageresourcestore/errors"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

// State describes retrieval and persistence methods for resource.
type State interface {
	// AddResourcesBeforeApplication adds the details of which resource
	// revision to use before the application exists in the model. The
	// charm and resource metadata must exist.
	AddResourcesBeforeApplication(ctx context.Context, arg resource.AddResourcesBeforeApplicationArgs) ([]coreresource.UUID, error)

	// DeleteApplicationResources removes all associated resources of a given
	// application identified by applicationID.
	DeleteApplicationResources(ctx context.Context, applicationID coreapplication.ID) error

	// DeleteResourcesAddedBeforeApplication removes all resources for the
	// given resource UUIDs. These resource UUIDs must have been returned
	// by AddResourcesBeforeApplication.
	DeleteResourcesAddedBeforeApplication(ctx context.Context, resUUIDs []coreresource.UUID) error

	// DeleteUnitResources deletes the association of resources with a specific
	// unit.
	DeleteUnitResources(ctx context.Context, uuid coreunit.UUID) error

	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource name.
	GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (coreresource.UUID, error)

	// GetResourceUUIDByApplicationAndResourceName returns the UUID of the
	// application resource specified by natural key of application and resource
	// name.
	//
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ApplicationNotFound] is returned if the
	//     application is not found.
	//   - [resourceerrors.ResourceNotFound] if no resource with name exists for
	//     given application.
	GetResourceUUIDByApplicationAndResourceName(ctx context.Context, appName, resName string) (coreresource.UUID, error)

	// ListResources returns the list of resource for the given application.
	ListResources(ctx context.Context, applicationID coreapplication.ID) (coreresource.ApplicationResources, error)

	// GetResource returns the identified resource.
	GetResource(ctx context.Context, resourceUUID coreresource.UUID) (coreresource.Resource, error)

	// GetResourcesByApplicationID returns the list of resource for the given application.
	//
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ApplicationNotFound] if the application ID is not an
	//     existing one.
	//
	// If the application exists but doesn't have any resources, no error are
	// returned, the result just contains an empty list.
	GetResourcesByApplicationID(ctx context.Context, applicationID coreapplication.ID) ([]coreresource.Resource, error)

	// GetResourceType finds the type of the given resource from the resource table.
	//
	// The following error types can be expected to be returned:
	//   - [resourceerrors.ResourceNotFound] if the resource UUID cannot be
	//     found.
	GetResourceType(ctx context.Context, resourceUUID coreresource.UUID) (charmresource.Type, error)

	// RecordStoredResource records a stored resource along with who retrieved it.
	//
	// If recording a stored blob for a resource that already has a blob associated
	// with it, this association is removed and the hash of this blob returned in
	// droppedHash. If there was no blob associated, droppedHash is empty.
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

	// UpdateResourceRevisionAndDeletePriorVersion updates the revision of resource
	// to a new version. Increments charm modified version for the application to
	// trigger use of the new resource revision by the application. Any stored
	// resource will be deleted from the DB. The droppedHash will be returned to aid
	// in removing from the object store.
	UpdateResourceRevisionAndDeletePriorVersion(
		ctx context.Context,
		arg resource.UpdateResourceRevisionArgs,
		resType charmresource.Type,
	) (string, error)
}

type ResourceStoreGetter interface {
	// GetResourceStore returns the appropriate  ResourceStore for the
	// given resource type.
	GetResourceStore(context.Context, charmresource.Type) (coreresourcestore.ResourceStore, error)
}

const (
	// applicationSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid application regexp.
	applicationSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
)

var (
	validApplication = regexp.MustCompile("^" + applicationSnippet + "$")
)

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

// GetResourceUUIDByApplicationAndResourceName returns the ID of the application
// resource specified by natural key of application and resource name.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationNotFound] is returned if the application is
//     not found.
//   - [resourceerrors.ResourceNotFound] if no resource with name exists for
//     given application.
//   - [resourceerrors.ResourceNameNotValid] if no resource name is provided
//     in the args.
//   - [resourceerrors.ApplicationNameNotValid] if the application name is
//     invalid.
func (s *Service) GetResourceUUIDByApplicationAndResourceName(
	ctx context.Context,
	appName, resName string,
) (coreresource.UUID, error) {
	if resName == "" {
		return "", resourceerrors.ResourceNameNotValid
	}
	if !isValidApplicationName(appName) {
		return "", resourceerrors.ApplicationNameNotValid
	}
	return s.st.GetResourceUUIDByApplicationAndResourceName(ctx, appName, resName)
}

// ListResources returns the resource data for the given application including
// application, unit and repository resource data. Unit data is only included
// for machine units. Repository resource data is included if it exists.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationIDNotValid] is returned if the application ID
//     is not valid.
//   - [resourceerrors.ApplicationNotFound] when the specified application
//     does not exist.
//
// No error is returned if the provided application has no resource.
func (s *Service) ListResources(
	ctx context.Context,
	applicationID coreapplication.ID,
) (coreresource.ApplicationResources, error) {
	if err := applicationID.Validate(); err != nil {
		return coreresource.ApplicationResources{}, errors.Errorf("%w: %w", err, resourceerrors.ApplicationIDNotValid)
	}
	return s.st.ListResources(ctx, applicationID)
}

// GetResourcesByApplicationID retrieves resources associated with a specific application ID.
// Returns a slice of resources or an error if the operation fails.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationIDNotValid] is returned if the application ID
//     is not valid.
//   - [resourceerrors.ApplicationNotFound] is returned if the application ID
//     is not an existing one.
//
// If the application doesn't have any resources, no error are
// returned, the result just contain an empty list.
func (s *Service) GetResourcesByApplicationID(ctx context.Context, applicationID coreapplication.ID) ([]coreresource.Resource,
	error) {
	if err := applicationID.Validate(); err != nil {
		return nil, errors.Errorf("%w: %w", err, resourceerrors.ApplicationIDNotValid)
	}
	return s.st.GetResourcesByApplicationID(ctx, applicationID)
}

// GetResource returns the identified application resource.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) GetResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (coreresource.Resource, error) {
	if err := resourceUUID.Validate(); err != nil {
		return coreresource.Resource{}, errors.Errorf("application id: %w", err)
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

	if args.RetrievedBy != "" && args.RetrievedByType == coreresource.Unknown {
		return resourceerrors.RetrievedByTypeNotValid
	}

	if err := args.Origin.Validate(); err != nil {
		return errors.Errorf("resource origin: %w", err)
	}
	if args.Origin == charmresource.OriginUpload && args.Revision != -1 {
		return errors.Errorf(
			"resource with origin upload must have revision -1, found %d: %w",
			args.Revision, resourceerrors.ResourceRevisionNotValid,
		)
	}
	if args.Origin == charmresource.OriginStore && args.Revision < 0 {
		return errors.Errorf(
			"resource with origin upload must have positive revision, found %d, %w",
			args.Revision, resourceerrors.ResourceRevisionNotValid,
		)
	}

	res, err := s.st.GetResource(ctx, args.ResourceUUID)
	if err != nil {
		return errors.Errorf("getting resource: %w", err)
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
	if errors.Is(err, objectstoreerrors.ObjectAlreadyExists) ||
		errors.Is(err, containerimageresourcestoreerrors.ContainerImageMetadataAlreadyStored) {
		return resourceerrors.StoredResourceAlreadyExists
	} else if err != nil {
		return errors.Errorf("putting resource %q in store: %w", res.Name, err)
	}
	defer func() {
		// If any subsequent operation fails, remove the resource blob.
		if err != nil {
			rErr := store.Remove(ctx, path)
			if rErr != nil {
				s.logger.Errorf(context.TODO(), "removing resource %s from store: %w", rErr)
			}
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
			Origin:                        args.Origin,
			Revision:                      args.Revision,
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
			s.logger.Errorf(context.TODO(), "failed to remove resource with ID %s from the store", droppedHash)
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
) (coreresource.Resource, io.ReadCloser, error) {
	if err := resourceUUID.Validate(); err != nil {
		return coreresource.Resource{}, nil, errors.Errorf("resource id: %w", err)
	}

	res, err := s.st.GetResource(ctx, resourceUUID)
	if err != nil {
		return coreresource.Resource{}, nil, err
	}

	store, err := s.resourceStoreGetter.GetResourceStore(ctx, res.Type)
	if err != nil {
		return coreresource.Resource{}, nil, errors.Errorf("getting resource store for %s: %w", res.Type.String(), err)
	}

	// TODO(aflynn): ideally this would be finding the resource via the
	// resources storageID, however the object store does not currently have a
	// method for this.
	reader, _, err := store.Get(ctx, blobPath(resourceUUID, res.Fingerprint.String()))
	if errors.Is(err, objectstoreerrors.ObjectNotFound) ||
		errors.Is(err, containerimageresourcestoreerrors.ContainerImageMetadataNotFound) {
		return coreresource.Resource{}, nil, resourceerrors.StoredResourceNotFound
	} else if err != nil {
		return coreresource.Resource{}, nil, errors.Errorf("getting resource from store: %w", err)
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

// SetRepositoryResources updates the last available revision of resources
// from charm repository for a specific application.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationIDNotValid] is returned if the Application ID is not valid.
//   - [resourceerrors.CharmIDNotValid] is returned if the charm ID is not valid.
//   - [resourceerrors.ArgumentNotValid] is returned if LastPolled is zero,
//     if the length of Info is zero or if any info are invalid.
//   - [resourceerrors.ApplicationNotFound] if the specified application does
//     not exist.
func (s *Service) SetRepositoryResources(
	ctx context.Context,
	args resource.SetRepositoryResourcesArgs,
) error {
	if err := args.ApplicationID.Validate(); err != nil {
		return errors.Errorf("%w: %w", resourceerrors.ApplicationIDNotValid, err)
	}
	if err := args.CharmID.Validate(); err != nil {
		return errors.Errorf("%w: %w", resourceerrors.CharmIDNotValid, err)
	}
	if len(args.Info) == 0 {
		return errors.Errorf("empty Info: %w", resourceerrors.ArgumentNotValid)
	}
	for _, info := range args.Info {
		if err := info.Validate(); err != nil {
			return errors.Errorf("%w: resource: %w", resourceerrors.ArgumentNotValid, err)
		}
	}
	if args.LastPolled.IsZero() {
		return errors.Errorf("zero LastPolled: %w", resourceerrors.ArgumentNotValid)
	}
	return s.st.SetRepositoryResources(ctx, args)
}

// AddResourcesBeforeApplication adds the details of which resource
// revision to use before the application exists in the model. The
// charm and resource metadata must exist. These resources are resolved
// when the application is created using the returned Resource UUIDs.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.CharmIDNotValid] is returned if the Charm ID is not valid.
//   - [resourceerrors.ArgumentNotValid] is returned if the origin is store and the revision
//     is empty.
//   - [resourceerrors.ResourceNameNotValid] is returned if resource name is empty.
//   - [resourceerrors.ApplicationNameNotFound] if the specified application does
//     not exist.
func (s *Service) AddResourcesBeforeApplication(ctx context.Context, arg resource.AddResourcesBeforeApplicationArgs) ([]coreresource.UUID, error) {
	if err := arg.CharmUUID.Validate(); err != nil {
		return nil, errors.Errorf("%w: %w", resourceerrors.CharmIDNotValid, err)
	}
	if !isValidApplicationName(arg.ApplicationName) {
		return nil, errors.Errorf("application name : %w", resourceerrors.ApplicationNameNotValid)
	}
	for _, res := range arg.ResourceDetails {
		if res.Name == "" {
			return nil, errors.Errorf("resource name is empty: %w", resourceerrors.ResourceNameNotValid)
		}
		if res.Origin == charmresource.OriginStore && res.Revision == nil {
			return nil, errors.Errorf("revision is empty for store resource: %w", resourceerrors.ArgumentNotValid)
		}
		if res.Origin == charmresource.OriginUpload && res.Revision != nil {
			return nil, errors.Errorf("revision is set for upload resource: %w", resourceerrors.ArgumentNotValid)
		}
	}
	resourceUUIDs, err := s.st.AddResourcesBeforeApplication(ctx, arg)
	if err != nil {
		return nil, errors.Errorf("failed to add resources: %w", err)
	}
	return resourceUUIDs, nil
}

// UpdateResourceRevision updates the revision of a store resource to a new
// version. Increments charm modified version for the application to
// trigger use of the new resource revision by the application. To allow for
// a resource upgrade, the current resource blob is removed.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceUUIDNotValid] is returned if the Resource ID is not valid.
func (s *Service) UpdateResourceRevision(ctx context.Context, arg resource.UpdateResourceRevisionArgs) error {
	if err := arg.ResourceUUID.Validate(); err != nil {
		return errors.Errorf("%w: %w", resourceerrors.ResourceUUIDNotValid, err)
	}

	resType, err := s.st.GetResourceType(ctx, arg.ResourceUUID)
	if err != nil {
		return err
	}

	droppedHash, err := s.st.UpdateResourceRevisionAndDeletePriorVersion(
		ctx,
		resource.UpdateResourceRevisionArgs{
			ResourceUUID: arg.ResourceUUID,
			Revision:     arg.Revision,
		},
		resType,
	)
	if err != nil {
		return err
	}

	if droppedHash != "" {
		store, err := s.resourceStoreGetter.GetResourceStore(ctx, resType)
		if err != nil {
			return errors.Errorf("getting resource store for %s: %w", resType.String(), err)
		}

		err = store.Remove(ctx, blobPath(arg.ResourceUUID, droppedHash))
		if err != nil {
			s.logger.Errorf(context.TODO(), "failed to remove resource with ID %s from the store", arg.ResourceUUID)
		}
	}
	return err
}

// DeleteResourcesAddedBeforeApplication removes all resources for the
// given resource UUIDs. These resource UUIDs must have been returned
// by AddResourcesBeforeApplication.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceUUIDNotValid] is returned if the Resource ID is not valid.
func (s *Service) DeleteResourcesAddedBeforeApplication(ctx context.Context, resUUIDs []coreresource.UUID) error {
	for _, resUUID := range resUUIDs {
		if err := resUUID.Validate(); err != nil {
			return errors.Errorf("%w: %w", resourceerrors.ResourceUUIDNotValid, err)
		}
	}
	return s.st.DeleteResourcesAddedBeforeApplication(ctx, resUUIDs)
}

// Store the resource with a path made up of the UUID and the resource
// hash, this ensures that different resource blobs are stored in
// different locations.
func blobPath(uuid coreresource.UUID, hash string) string {
	return "resource-" + uuid.String() + "#" + hash
}

// isValidApplicationName returns whether name is a valid application name.
func isValidApplicationName(name string) bool {
	return validApplication.MatchString(name)
}
