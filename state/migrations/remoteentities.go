// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
)

// MigrationRemoteEntity represents a state.RemoteEntity
// Point of use interface to enable better encapsulation.
type MigrationRemoteEntity interface {
	ID() string
	Token() string
	Macaroon() string
}

// RemoteEntitiesSource defines an inplace usage for reading all the remote
// entities.
type RemoteEntitiesSource interface {
	AllRemoteEntities() ([]MigrationRemoteEntity, error)
}

// RemoteEntitiesModel defines an inplace usage for adding a remote entity
// to a model.
type RemoteEntitiesModel interface {
	AddRemoteEntity(description.RemoteEntityArgs) description.RemoteEntity
}

// ExportRemoteEntities describes a way to execute a migration for exporting
// remote entities.
type ExportRemoteEntities struct{}

// Execute the migration of the remote entities using typed interfaces, to
// ensure we don't loose any type safety.
// This doesn't conform to an interface because go doesn't have generics, but
// when this does arrive this would be an excellent place to use them.
func (ExportRemoteEntities) Execute(src RemoteEntitiesSource, dst RemoteEntitiesModel) error {
	entities, err := src.AllRemoteEntities()
	if err != nil {
		return errors.Trace(err)
	}
	for _, entity := range entities {
		// Despite remote entities having a member for macaroon,
		// they are not exported.
		dst.AddRemoteEntity(description.RemoteEntityArgs{
			ID:    entity.ID(),
			Token: entity.Token(),
		})
	}
	return nil
}
