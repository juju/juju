// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"maps"
	"slices"

	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/life"
)

// nameAndUUID is an agnostic container for the pair of
// `uuid` and `name` columns.
type nameAndUUID struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// offerEndpoint represent a row in the offer_endpoint table.
type offerEndpoint struct {
	OfferUUID    string `db:"offer_uuid"`
	EndpointUUID string `db:"endpoint_uuid"`
}

// uuid is an agnostic container for a `uuid` column.
type uuid struct {
	UUID string `db:"uuid"`
}

// name is an agnostic container for a `name` column.
type name struct {
	Name string `db:"name"`
}

type offerAndApplicationUUID struct {
	UUID            string `db:"uuid"`
	ApplicationUUID string `db:"application_uuid"`
}

// offerDetail contains the data necessary for create
// OfferDetail structures
type offerDetail struct {
	OfferUUID              string `db:"offer_uuid"`
	OfferName              string `db:"offer_name"`
	ApplicationName        string `db:"application_name"`
	ApplicationDescription string `db:"application_description"`

	// CharmLocator parts
	CharmName         string                    `db:"charm_name"`
	CharmRevision     int                       `db:"charm_revision"`
	CharmSource       charm.CharmSource         `db:"charm_source"`
	CharmArchitecture architecture.Architecture `db:"charm_architecture"`

	// OfferEndpoint parts
	EndpointName      string             `db:"endpoint_name"`
	EndpointRole      charm.RelationRole `db:"endpoint_role"`
	EndpointInterface string             `db:"endpoint_interface"`
}

type offerFilter struct {
	OfferName              string `db:"offer_name"`
	ApplicationName        string `db:"application_name"`
	ApplicationDescription string `db:"application_description"`
	EndpointName           string `db:"endpoint_name"`
	Interface              string `db:"endpoint_interface"`
	Role                   string `db:"endpoint_role"`
}

type offerDetails []offerDetail

func (o offerDetails) TransformToOfferDetails() []*crossmodelrelation.OfferDetail {
	converted := make(map[string]*crossmodelrelation.OfferDetail, 0)
	for _, details := range o {
		found, ok := converted[details.OfferUUID]
		if ok {
			// TODO  - ensure endpoints unique
			// Seen this offer before, add more endpoints, and keep going.
			found.Endpoints = append(found.Endpoints, crossmodelrelation.OfferEndpoint{
				Name:      details.EndpointName,
				Role:      details.EndpointRole,
				Interface: details.EndpointInterface,
			})
			converted[details.OfferUUID] = found
			continue
		}
		// New offer, add with one endpoint.
		found = &crossmodelrelation.OfferDetail{
			OfferUUID:              details.OfferUUID,
			OfferName:              details.OfferName,
			ApplicationName:        details.ApplicationName,
			ApplicationDescription: details.ApplicationDescription,
			CharmLocator: charm.CharmLocator{
				Name:         details.CharmName,
				Revision:     details.CharmRevision,
				Source:       details.CharmSource,
				Architecture: details.CharmArchitecture,
			},
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      details.EndpointName,
					Role:      details.EndpointRole,
					Interface: details.EndpointInterface,
				},
			},
		}
		converted[details.OfferUUID] = found
	}

	return slices.Collect(maps.Values(converted))
}

type applicationDetails struct {
	UUID      string    `db:"uuid"`
	Name      string    `db:"name"`
	CharmUUID string    `db:"charm_uuid"`
	LifeID    life.Life `db:"life_id"`
	SpaceUUID string    `db:"space_uuid"`
}

type countResult struct {
	Count int `db:"count"`
}

// setCharmState is used to set the charm.
type setCharmState struct {
	UUID          string `db:"uuid"`
	ReferenceName string `db:"reference_name"`
	SourceID      int    `db:"source_id"`
}

// setCharmMetadata is used to set the metadata of a charm.
// This includes the setting of the LXD profile.
type setCharmMetadata struct {
	CharmUUID   string `db:"charm_uuid"`
	Name        string `db:"name"`
	Description string `db:"description"`
}

// setCharmRelation is used to set the relations of a charm.
type setCharmRelation struct {
	UUID      string `db:"uuid"`
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"name"`
	RoleID    int    `db:"role_id"`
	Interface string `db:"interface"`
	Optional  bool   `db:"optional"`
	Capacity  int    `db:"capacity"`
	ScopeID   int    `db:"scope_id"`
}

type remoteApplicationOfferer struct {
	// UUID is the unique identifier for this remote application offerer.
	UUID string `db:"uuid"`
	// LifeID is the life state of the remote application offerer.
	LifeID life.Life `db:"life_id"`
	// ApplicationUUID is the unique identifier for the application
	// that is being offered.
	ApplicationUUID string `db:"application_uuid"`
	// OfferUUID is the offer uuid that ties both the offerer and consumer
	// together.
	OfferUUID string `db:"offer_uuid"`
	// Version is the version of the remote application offerer.
	Version uint64 `db:"version"`
	// OffererControllerUUID is the unique identifier for the controller
	// that is offering this application.
	OffererControllerUUID sql.Null[string] `db:"offerer_controller_uuid"`
	// OffererModelUUID is the unique identifier for the model
	// that is offering this application.
	OffererModelUUID string `db:"offerer_model_uuid"`
	// Macaroon is the serialized macaroon that can be used to
	// authenticate to the offerer controller.
	Macaroon []byte `db:"macaroon"`
}
