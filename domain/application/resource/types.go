// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"
	"time"

	"github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/charm/resource"
)

// IncrementCharmModifiedVersionType is the argument type for incrementing
// CharmModifiedVersion or not.
//
// A change in CharmModifiedVersion triggers the uniter to run the upgrade_charm
// hook (and config hook). Increment required for a running unit to pick up
// new resources from `attach` or when a charm is upgraded without a new charm
// revision.
type IncrementCharmModifiedVersionType bool

const (
	// IncrementCharmModifiedVersion means CharmModifiedVersion needs to be incremented.
	IncrementCharmModifiedVersion IncrementCharmModifiedVersionType = true
	// DoNotIncrementCharmModifiedVersion means CharmModifiedVersion should not be incremented.
	DoNotIncrementCharmModifiedVersion IncrementCharmModifiedVersionType = false
)

// ApplicationResources contains the list of resources for the application
// and all its units using file type resources. It also contains resources
// available from the repository for this application based on the channel
// used by the application's charm. Repository resources can be used to
// determine if an application's resources may be refreshed to newer versions.
type ApplicationResources struct {
	// Resources are the current version of the resource for the application that
	// resource-get will retrieve.
	Resources []Resource

	// RepositoryResources provides the resource info from charm hub
	// for each of the application's resources. The information from
	// hub charm hub is current as of the last time the charm hub
	// was polled. Each entry here corresponds to the same indexed entry
	// in the Resources field. An entry may be empty if data has not
	// yet been retrieve from the repository.
	RepositoryResources []resource.Resource

	// UnitResources reports the currently-in-use version of file type
	// resources for each unit.
	UnitResources []UnitResources
}

// Resource defines a single resource within a Juju model.
//
// Each application will have exactly the same resources associated
// with it as are defined in the charm's metadata, no more, no less.
// When associated with the application the resource may have additional
// information associated with it.
//
// A resource may be a "placeholder", meaning it is only partially
// populated before an upload (whether local or from the charm store).
// In that case the following fields are not set:
//
//	Timestamp, RetrievedBy, RetrievedByType
//
// For "upload" placeholders, the following additional fields are
// not set:
//
//	Fingerprint, Size
type Resource struct {
	resource.Resource

	// UUID uniquely identifies a resource within the model.
	UUID coreresource.UUID

	// ApplicationID identifies the application for the resource.
	ApplicationID application.ID

	// RetrievedBy is the name of who added the resource to the controller.
	// The name is a username if the resource is uploaded from the cli
	// by a specific user. If the resource is downloaded from a repository,
	// the ID of the unit which triggered the download is used.
	RetrievedBy string

	// RetrievedByType indicates what type of value the RetrievedBy name is:
	// application, username or unit.
	RetrievedByType RetrievedByType

	// Timestamp indicates when this resource was added to the model in
	// the case of applications or when this resource was loaded by a unit.
	Timestamp time.Time
}

// RetrievedByType indicates what the RetrievedBy name represents.
type RetrievedByType string

const (
	Unknown     RetrievedByType = "unknown"
	Application RetrievedByType = "application"
	Unit        RetrievedByType = "unit"
	User        RetrievedByType = "user"
)

// UnitResources contains the list of resources used by a unit.
type UnitResources struct {
	// ID is the ID of the unit.
	ID unit.UUID

	// Resources are the resource versions currently in use by this unit.
	Resources []Resource
}

// GetApplicationResourceIDArgs holds the arguments for the
// GetApplicationResourceID method.
type GetApplicationResourceIDArgs struct {
	ApplicationID application.ID
	Name          string
}

// SetResourceArgs holds the arguments for the SetResource method.
type SetResourceArgs struct {
	ApplicationID  application.ID
	SuppliedBy     string
	SuppliedByType RetrievedByType
	Resource       resource.Resource
	Reader         io.Reader
	Increment      IncrementCharmModifiedVersionType
}

// SetUnitResourceArgs holds the arguments for the SetUnitResource method.
type SetUnitResourceArgs struct {
	ResourceID      coreresource.UUID
	RetrievedBy     string
	RetrievedByType RetrievedByType
	UnitID          unit.UUID
}

// SetUnitResourceResult is the result data from setting a unit's resource.
type SetUnitResourceResult struct {
	// UUID uniquely identifies the unit resource within the model.
	UUID coreresource.UUID
	// Timestamp indicates when the unit started using resource.
	Timestamp time.Time
}

// SetRepositoryResourcesArgs holds the arguments for the
// SetRepositoryResources method.
type SetRepositoryResourcesArgs struct {
	// ApplicationID is the id of the application having these resources.
	ApplicationID application.ID
	// Info is a slice of resource data received from the repository.
	Info []resource.Resource
	// LastPolled indicates when the resource data was last polled.
	LastPolled time.Time
}

// ResourceStorageUUID is a UUID used to reference the resource in storage.
type ResourceStorageUUID interface {
	String() string
	Validate() error
}
