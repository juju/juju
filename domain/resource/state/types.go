// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/charm"
	coreresource "github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// resourceAndAppName represents the resource name and app name, this can be
// used as an identifier for a resource.
type resourceAndAppName struct {
	ApplicationName string `db:"application_name"`
	ResourceName    string `db:"resource_name"`
}

// resourceIdentity represents the unique identity of a resource within an
// application.
type resourceIdentity struct {
	UUID            string `db:"uuid"`
	ApplicationUUID string `db:"application_uuid"`
	Name            string `db:"name"`
}

// localUUID represents a unique identifier.
type localUUID struct {
	UUID string `db:"uuid"`
}

// charmUUID represents the unique identifier of a charm.
type charmUUID struct {
	UUID string `db:"uuid"`
}

type applicationUUID struct {
	UUID string `db:"application_uuid"`
}

// uuids is a slice of resource UUIDs.
type uuids []string

// applicationResource represents a link between an application and a resource.
type applicationResource struct {
	ResourceUUID    string `db:"resource_uuid"`
	ApplicationUUID string `db:"application_uuid"`
}

// resourceKind is the kind of the resource, e.g. file or oci-image.
type resourceKind struct {
	Name string `db:"kind_name"`
	UUID string `db:"uuid"`
}

// resourceView represents the view model for a resource entity. It contains
// all fields from v_application_resource
type resourceView struct {
	UUID            string    `db:"uuid"`
	ApplicationUUID string    `db:"application_uuid"`
	ApplicationName string    `db:"application_name"`
	Name            string    `db:"name"`
	CreatedAt       time.Time `db:"created_at"`
	Revision        *int      `db:"revision"`
	OriginType      string    `db:"origin_type"`
	RetrievedBy     string    `db:"retrieved_by"`
	RetrievedByType string    `db:"retrieved_by_type"`
	Path            string    `db:"path"`
	Description     string    `db:"description"`
	Kind            string    `db:"kind_name"`
	Size            int64     `db:"size"`
	SHA384          string    `db:"sha384"`
	State           string    `db:"state"`
}

// toCharmResource converts the resourceView struct to a
// charmresource.Resource, populating its fields accordingly.
func (rv resourceView) toCharmResource() (charmresource.Resource, error) {
	kind, err := charmresource.ParseType(rv.Kind)
	if err != nil {
		return charmresource.Resource{}, errors.Errorf("converting resource type: %w", err)
	}
	origin, err := charmresource.ParseOrigin(rv.OriginType)
	if err != nil {
		return charmresource.Resource{}, errors.Errorf("converting origin type: %w", err)
	}
	var fingerprint charmresource.Fingerprint
	if rv.SHA384 != "" {
		fingerprint, err = charmresource.ParseFingerprint(rv.SHA384)
		if err != nil {
			return charmresource.Resource{}, errors.Errorf("converting resource fingerprint: %w", err)
		}
	}
	var revision int
	if rv.Revision != nil {
		revision = *rv.Revision
	}

	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        rv.Name,
			Type:        kind,
			Path:        rv.Path,
			Description: rv.Description,
		},
		Origin:      origin,
		Revision:    revision,
		Fingerprint: fingerprint,
		Size:        rv.Size,
	}, nil
}

// toResource converts a resourceView object to a resource.Resource object
// including metadata and timestamps.
func (rv resourceView) toResource() (coreresource.Resource, error) {
	charmRes, err := rv.toCharmResource()
	if err != nil {
		return coreresource.Resource{}, errors.Capture(err)
	}
	return coreresource.Resource{
		UUID:            coreresource.UUID(rv.UUID),
		Resource:        charmRes,
		ApplicationName: rv.ApplicationName,
		RetrievedBy:     rv.RetrievedBy,
		Timestamp:       rv.CreatedAt,
	}, nil
}

// unitResource represents the mapping of a resource to a unit.
type unitResource struct {
	ResourceUUID string    `db:"resource_uuid"`
	UnitUUID     string    `db:"unit_uuid"`
	AddedAt      time.Time `db:"added_at"`
}

type applicationNameAndID struct {
	ApplicationID coreapplication.UUID `db:"uuid"`
	Name          string             `db:"name"`
}

type applicationID struct {
	ID coreapplication.UUID `db:"uuid"`
}

// charmResource contains the identifiers of the charm resource in the
// v_charm_resource view.
type charmResource struct {
	CharmUUID    string `db:"charm_uuid"`
	ResourceName string `db:"name"`
	Kind         string `db:"kind"`
}

// getApplicationAndCharmID gets the application and charm ID from the
// application table using the application name.
type getApplicationAndCharmID struct {
	ApplicationID coreapplication.UUID `db:"uuid"`
	CharmID       charm.ID           `db:"charm_uuid"`
	Name          string             `db:"name"`
}

// kubernetesApplicationResource represents the mapping of a resource to a unit.
type kubernetesApplicationResource struct {
	ResourceUUID string    `db:"resource_uuid"`
	AddedAt      time.Time `db:"added_at"`
}

type storedFileResource struct {
	ObjectStoreUUID string `db:"store_uuid"`
	ResourceUUID    string `db:"resource_uuid"`
	Size            int64  `db:"size"`
	SHA384          string `db:"sha384"`
}

type storedContainerImageResource struct {
	StorageKey   string `db:"store_storage_key"`
	ResourceUUID string `db:"resource_uuid"`
	Size         int64  `db:"size"`
	Hash         string `db:"sha384"`
}

// unitUUIDAndName represents an unit with uuid and name
type unitUUIDAndName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// addPendingResource holds the data required to add a pending
// resource into the resource table.
type addPendingResource struct {
	UUID      string    `db:"uuid"`
	CharmUUID string    `db:"charm_uuid"`
	Name      string    `db:"charm_resource_name"`
	Revision  *int      `db:"revision"`
	Origin    string    `db:"origin_type_name"`
	State     string    `db:"state_name"`
	CreatedAt time.Time `db:"created_at"`
}

// linkResourceApplication represents a row in the pending_application_resource
// table.
type linkResourceApplication struct {
	ResourceUUID    string `db:"resource_uuid"`
	ApplicationName string `db:"application_name"`
}

// hash represents the hash value from a stored resource blob.
type hash struct {
	Hash string `db:"sha384"`
}

// setResource is used to set resource rows in the resource table.
type setResource struct {
	UUID         string    `db:"uuid"`
	CharmUUID    string    `db:"charm_uuid"`
	Name         string    `db:"charm_resource_name"`
	Revision     *int      `db:"revision"`
	OriginTypeId int       `db:"origin_type_id"`
	StateID      int       `db:"state_id"`
	CreatedAt    time.Time `db:"created_at"`
}

// addResource is used to set resource rows in the resource table.
type addResource struct {
	UUID      string    `db:"uuid"`
	CharmUUID string    `db:"charm_uuid"`
	Name      string    `db:"charm_resource_name"`
	Revision  *int      `db:"revision"`
	Origin    string    `db:"origin_type_name"`
	State     string    `db:"state_name"`
	CreatedAt time.Time `db:"created_at"`
}

type resourceCharmData struct {
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"charm_resource_name"`
}

type uuidOriginAndRevision struct {
	UUID     coreresource.UUID
	Origin   charmresource.Origin
	Revision int
}

type charmModifiedVersion struct {
	Version uint64 `db:"charm_modified_version"`
}
