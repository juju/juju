// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

// resourceIdentity represents the unique identity of a resource within an
// application.
type resourceIdentity struct {
	UUID            string `db:"uuid"`
	ApplicationUUID string `db:"application_uuid"`
	Name            string `db:"name"`
}

// resourceView represents the view model for a resource entity. It contains
// all fields from v_resource table view.
type resourceView struct {
	UUID            string    `db:"uuid"`
	ApplicationUUID string    `db:"application_uuid"`
	Name            string    `db:"name"`
	CreatedAt       time.Time `db:"created_at"`
	Revision        int       `db:"revision"`
	OriginTypeId    int       `db:"origin_type_id"`
	RetrievedBy     string    `db:"retrieved_by"`
	RetrievedByType string    `db:"retrieved_by_type"`
	Path            string    `db:"path"`
	Description     string    `db:"description"`
	KindId          int       `db:"kind_id"`
}

// toCharmResource converts the resourceView struct to a
// charmresource.Resource, populating its fields accordingly.
func (rv resourceView) toCharmResource() charmresource.Resource {
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        rv.Name,
			Type:        charmresource.Type(rv.KindId),
			Path:        rv.Path,
			Description: rv.Description,
		},
		Origin:   charmresource.Origin(rv.OriginTypeId),
		Revision: rv.Revision,
		// todo(gfouillet): deal with fingerprint & size
		Fingerprint: charmresource.Fingerprint{},
		Size:        0,
	}
}

// toResource converts a resourceView object to a resource.Resource object
// including metadata and timestamps.
func (rv resourceView) toResource() resource.Resource {
	return resource.Resource{
		Resource:        rv.toCharmResource(),
		UUID:            coreresource.UUID(rv.UUID),
		ApplicationID:   coreapplication.ID(rv.ApplicationUUID),
		RetrievedBy:     rv.RetrievedBy,
		RetrievedByType: resource.RetrievedByType(rv.RetrievedByType),
		Timestamp:       rv.CreatedAt,
	}
}

// unitResource represents the mapping of a resource to a unit.
type unitResource struct {
	ResourceUUID string    `db:"resource_uuid"`
	UnitUUID     string    `db:"unit_uuid"`
	AddedAt      time.Time `db:"added_at"`
}

// unitNameAndUUID store the name & uuid of a unit
type unitNameAndUUID struct {
	UnitUUID coreunit.UUID `db:"uuid"`
	Name     coreunit.Name `db:"name"`
}

type applicationNameAndID struct {
	ApplicationID coreapplication.ID `db:"uuid"`
	Name          string             `db:"name"`
}
