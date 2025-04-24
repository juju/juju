// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	corecloud "github.com/juju/juju/core/cloud"
	coremodel "github.com/juju/juju/core/model"
)

// These structs represent the persistent cloud credential entity schema in the database.

type modelUUID struct {
	UUID coremodel.UUID `db:"uuid"`
}
type cloudCredentialWithAttribute struct {
	CloudUUID       corecloud.UUID `db:"cloud_uuid"`
	CloudRegionName string         `db:"cloud_region_name"`
	AuthType        string         `db:"auth_type"`
	AttributeKey    string         `db:"attribute_key"`
	AttributeValue  string         `db:"attribute_value"`
}
