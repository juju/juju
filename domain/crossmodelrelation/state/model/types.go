// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"maps"
	"slices"

	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
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
