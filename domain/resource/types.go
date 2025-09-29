// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"
	"time"

	"github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/domain/application/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

// StateType indicates if a resource is available to be used on the
// controller or not.
type StateType string

func (s StateType) String() string {
	return string(s)
}

const (
	// StatePotential indicates that a resource refers to a version that could
	// potentially be downloaded from charmhub. Potential resources are used to
	// let users know a resource can be upgraded.
	StatePotential StateType = "potential"
	// StateAvailable indicates that a resource refers to an active resource on
	// the controller that may be used by applications and units.
	StateAvailable StateType = "available"
)

// GetApplicationResourceIDArgs holds the arguments for the
// GetApplicationResourceID method.
type GetApplicationResourceIDArgs struct {
	ApplicationID application.UUID
	Name          string
}

// SetRepositoryResourcesArgs holds the arguments for the
// SetRepositoryResources method.
type SetRepositoryResourcesArgs struct {
	// ApplicationID is the id of the application having these resources.
	ApplicationID application.UUID
	// CharmID is the unique identifier for a charm to update resources.
	CharmID corecharm.ID
	// Info is a slice of resource data received from the repository.
	Info []charmresource.Resource
	// LastPolled indicates when the resource data was last polled.
	LastPolled time.Time
}

// StoreResourceArgs holds the arguments for resource storage methods.
type StoreResourceArgs struct {
	// ResourceUUID is the unique identifier of the resource.
	ResourceUUID coreresource.UUID
	// Reader is a reader for the resource blob.
	Reader io.Reader
	// RetrievedBy is the identity of the entity that retrieved the resource.
	// This field is optional.
	RetrievedBy string
	// RetrievedByType is the type of entity that retrieved the resource. This
	// field is optional.
	RetrievedByType coreresource.RetrievedByType
	// Size is the size in bytes of the resource blob.
	Size int64
	// Fingerprint is the hash of the resource blob.
	Fingerprint charmresource.Fingerprint
}

// RecordStoredResourceArgs holds the arguments for record stored resource state
// method.
type RecordStoredResourceArgs struct {
	// ResourceUUID is the unique identifier of the resource.
	ResourceUUID coreresource.UUID
	// StorageID is the store ID of the resources' blob.
	StorageID coreresourcestore.ID
	// RetrievedBy is the identity of the entity that retrieved the resource.
	// This field is optional.
	RetrievedBy string
	// RetrievedByType is the type of entity that retrieved the resource. This
	// field is optional.
	RetrievedByType coreresource.RetrievedByType
	// ResourceType is the type of the resource
	ResourceType charmresource.Type
	// IncrementCharmModifiedVersion indicates weather the charm modified
	// version should be incremented or not.
	IncrementCharmModifiedVersion bool
	// Size is the size in bytes of the resource blob.
	Size int64
	// SHA384 is the hash of the resource blob.
	SHA384 string
}

// AddResourcesBeforeApplicationArgs holds arguments to indicate a resources revision or upload
// before the application has been created.
type AddResourcesBeforeApplicationArgs struct {
	// ApplicationName is the unique name of the application.
	ApplicationName string
	// CharmLocator is the unique identifier of the charm.
	CharmLocator charm.CharmLocator
	// ResourceDetails contains individual resource details.
	ResourceDetails []AddResourceDetails
}

// AddResourceDetails contains details of the resource to be added before
// the application exists.
type AddResourceDetails struct {
	// Name is resource name.
	Name string
	// Origin is where the resource comes from.
	Origin charmresource.Origin
	// Revision is a optional revision value, not required for uploaded resources.
	Revision *int
}

// UpdateUploadResourceArgs holds arguments to update the resource to
// expect a new blob to be uploaded.
type UpdateUploadResourceArgs struct {
	// ApplicationID is the ID of the application this resource belongs to.
	ApplicationID application.UUID
	// Name is the resource name.
	Name string
}

// StateUpdateUploadResourceArgs holds arguments for the state method to
// update the resource to expect a new blob to be uploaded.
type StateUpdateUploadResourceArgs struct {
	// ResourceType is the type of the resource
	ResourceType charmresource.Type
	// ResourceUUID is the unique identifier of the resource.
	ResourceUUID coreresource.UUID
}

// UpdateResourceRevisionArgs holds arguments to update a resource to have
// a new revision.
type UpdateResourceRevisionArgs struct {
	// ResourceUUID is the unique identifier of the resource.
	ResourceUUID coreresource.UUID
	// Revision is the revision of the resource to use.
	Revision int
}

// ImportResourcesArgs are the arguments for SetResource.
type ImportResourcesArgs []ImportResourcesArg

// ImportResourcesArg is a single argument for the ImportResources method.
type ImportResourcesArg struct {
	// ApplicationName is the name of the application these resources are
	// associated with.
	ApplicationName string
	// ApplicationResources are the available resources on the application.
	Resources []ImportResourceInfo
	// UnitResources contains information about the units using the resources in
	// ApplicationResources.
	UnitResources []ImportUnitResourceInfo
}

// ImportResourceInfo contains information about a single resource for the
// ImportResources method.
type ImportResourceInfo struct {
	// Name is the name of the resource.
	Name string
	// Origin identifies where the resource will come from.
	Origin charmresource.Origin
	// Revision is the charm store revision of the resource.
	Revision int
	// Timestamp is the time the resource was added to the model.
	Timestamp time.Time
}

// ImportUnitResourceInfo contains information about a single unit resource for the
// ImportResources method.
type ImportUnitResourceInfo struct {
	ImportResourceInfo

	// UnitName is the name of the unit using the resource.
	UnitName string
}

// ExportedResources holds all resources to be exported.
type ExportedResources struct {
	// Resources are the resources to be exported.
	Resources []coreresource.Resource
	// UnitResources are the unit resources to be exported.
	UnitResources []coreresource.UnitResources
}
