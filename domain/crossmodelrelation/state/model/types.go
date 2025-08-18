// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

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
