// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/names/v6"

	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/rpc/params"
)

// StorageEntities renders the storage instance classifications as
// [params.Entity] values, tagging each with its storage instance
// identifier. Returns nil when no instances are supplied.
func StorageEntities(instances []domainstorage.StorageInstanceClassification) []params.Entity {
	if len(instances) == 0 {
		return nil
	}
	entities := make([]params.Entity, 0, len(instances))
	for _, instance := range instances {
		entities = append(entities, params.Entity{
			Tag: names.NewStorageTag(instance.ID).String(),
		})
	}
	return entities
}
