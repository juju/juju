// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"github.com/juju/juju/core/database/schema"
)

const (
	tableModelConfig tableNamespaceID = iota + 1
)

// ModelDDL is used to create model databases.
func ModelDDL() *schema.Schema {
	patches := []func() schema.Patch{
		changeLogSchema,
		changeLogModelNamespace,
		modelConfig,
		changeLogTriggersForTable("model_config", "key", tableModelConfig),
		spacesSchema,
		objectStoreMetadataSchema,
	}

	schema := schema.New()
	for _, fn := range patches {
		schema.Add(fn())
	}
	return schema
}

func changeLogModelNamespace() schema.Patch {
	// Note: These should match exactly the values of the tableNamespaceID
	// constants above.
	return schema.MakePatch(`
INSERT INTO change_log_namespace VALUES
    (1, 'model_config', 'model config changes based on config key')
`)
}

func modelConfig() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE model_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`)
}

func spacesSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE provider_spaces (
    uuid            TEXT PRIMARY KEY,
    name            TEXT
);

CREATE TABLE spaces (
    uuid            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    is_public       BOOLEAN,
    provider_uuid   TEXT,
    CONSTRAINT      fk_lease_pin_lease
        FOREIGN KEY     (provider_uuid)
        REFERENCES      provider_spaces(uuid)
);

CREATE UNIQUE INDEX idx_spaces_uuid_name
ON spaces (uuid, name);
`)
}
