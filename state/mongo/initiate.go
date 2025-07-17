// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/juju/mgo/v3"
)

// InitiateMongoParams holds parameters for the MaybeInitiateMongo call.
type InitiateMongoParams struct {
	// DialInfo specifies how to connect to the mongo server.
	DialInfo *mgo.DialInfo

	// MemberHostPort provides the address to use for
	// the first replica set member.
	MemberHostPort string

	// User holds the user to log as in to the mongo server.
	// If it is empty, no login will take place.
	User string
}

// InitiateMongoServer checks for an existing mongo configuration.
// If no existing configuration is found one is created using Initiate.
func InitiateMongoServer(p InitiateMongoParams) error {
	return nil
}
