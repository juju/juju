// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
)

// MigrationRelationNetworks represents a state.RelationNetwork
// Point of use interface to enable better encapsulation.
type MigrationRelationNetworks interface {
	Id() string
	RelationKey() string
	CIDRS() []string
}

// RelationNetworksSource defines an inplace usage for reading all the relation
// networks.
type RelationNetworksSource interface {
	AllRelationNetworks() ([]MigrationRelationNetworks, error)
}

// RelationNetworksModel defines an inplace usage for adding a relation networks
// to a model.
type RelationNetworksModel interface {
	AddRelationNetwork(description.RelationNetworkArgs) description.RelationNetwork
}

// ExportRelationNetworks describes a way to execute a migration for
// exporting relation networks.
type ExportRelationNetworks struct{}

// Execute the migration of the relation networks using typed interfaces, to
// ensure we don't loose any type safety.
// This doesn't conform to an interface because go doesn't have generics, but
// when this does arrive this would be an execellent place to use them.
func (ExportRelationNetworks) Execute(src RelationNetworksSource, dst RelationNetworksModel) error {
	entities, err := src.AllRelationNetworks()
	if err != nil {
		return errors.Trace(err)
	}
	for _, entity := range entities {
		dst.AddRelationNetwork(description.RelationNetworkArgs{
			ID:          entity.Id(),
			RelationKey: entity.RelationKey(),
			CIDRS:       entity.CIDRS(),
		})
	}
	return nil
}
