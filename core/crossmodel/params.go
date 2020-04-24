// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"time"

	"github.com/juju/charm/v7"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/relation"
)

// ApplicationOfferAdminDetails represents the details about an
// application offer. Depending on the access permission of the
// user making the API call, and whether the call is "find" or "list",
// not all fields will be populated.
type ApplicationOfferDetails struct {
	// OfferName is the name of the offer
	OfferName string

	// ApplicationName is the application name to which the offer pertains.
	ApplicationName string

	// ApplicationDescription is the application description.
	ApplicationDescription string

	// OfferURL is the URL where the offer can be located.
	OfferURL string

	// CharmURL is the URL of the charm for the remote application.
	CharmURL string

	// Endpoints are the charm endpoints supported by the application.
	// TODO(wallyworld) - do not use charm.Relation here
	Endpoints []charm.Relation

	// Connects are the connections to the offer.
	Connections []OfferConnection

	// Users are the users able to access the offer.
	Users []OfferUserDetails
}

// OfferUserDetails holds the details about a user's access to an offer.
type OfferUserDetails struct {
	// UserName is the username of the user.
	UserName string

	// DisplayName is the display name of the user.
	DisplayName string

	// Access is the level of access to the offer.
	Access permission.Access
}

// OfferConnection holds details about a connection to an offer.
type OfferConnection struct {
	// SourceModelUUID is the UUID of the model hosting the offer.
	SourceModelUUID string

	// Username is the name of the user consuming the offer.
	Username string

	// RelationId is the id of the relation for this connection.
	RelationId int

	// Endpoint is the endpoint being connected to.
	Endpoint string

	// Status is the status of the offer connection.
	Status relation.Status

	// Message is the status message of the offer connection.
	Message string

	// Since is when the status value was last changed.
	Since *time.Time

	// IngressSubnets is the list of subnets from which traffic will originate.
	IngressSubnets []string
}
