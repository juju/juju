// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
)

// MigrationOfferConnection is an in-place representation of the
// state.OfferConnection
type MigrationOfferConnection interface {
	OfferUUID() string
	RelationId() int
	RelationKey() string
	UserName() string
	SourceModelUUID() string
}

// AllOfferConnectionSource defines an in-place usage for reading all the
// offer connections.
type AllOfferConnectionSource interface {
	AllOfferConnections() ([]MigrationOfferConnection, error)
}

// OfferConnectionSource composes all the interfaces to create a offer
// connection.
type OfferConnectionSource interface {
	AllOfferConnectionSource
}

// OfferConnectionModel defines an in-place usage for adding a offer connection
// to a model.
type OfferConnectionModel interface {
	AddOfferConnection(description.OfferConnectionArgs) description.OfferConnection
}

// ExportOfferConnections describes a way to execute a migration for exporting
// offer connections.
type ExportOfferConnections struct{}

// Execute the migration of the offer connections using typed interfaces, to
// ensure we don't loose any type safety.
// This doesn't conform to an interface because go doesn't have generics, but
// when this does arrive this would be an excellent place to use them.
func (m ExportOfferConnections) Execute(src OfferConnectionSource, dst OfferConnectionModel) error {
	offerConnections, err := src.AllOfferConnections()
	if err != nil {
		return errors.Trace(err)
	}

	for _, offerConnection := range offerConnections {
		m.addOfferConnection(dst, offerConnection)
	}
	return nil
}

func (m ExportOfferConnections) addOfferConnection(dst OfferConnectionModel, offer MigrationOfferConnection) {
	_ = dst.AddOfferConnection(description.OfferConnectionArgs{
		OfferUUID:       offer.OfferUUID(),
		RelationID:      offer.RelationId(),
		RelationKey:     offer.RelationKey(),
		SourceModelUUID: offer.SourceModelUUID(),
		UserName:        offer.UserName(),
	})
}
