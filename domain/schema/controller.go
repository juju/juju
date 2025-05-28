// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"embed"
	"fmt"
	"sort"

	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/domain/schema/controller/triggers"
)

//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/cloud-triggers.gen.go -package=triggers -tables=cloud,cloud_ca_cert,cloud_credential,cloud_credential_attribute
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/controller-triggers.gen.go -package=triggers -tables=controller_config,controller_node,external_controller
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/migration-triggers.gen.go -package=triggers -tables=model_migration_status,model_migration_minion_sync
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/upgrade-triggers.gen.go -package=triggers -tables=upgrade_info,upgrade_info_controller_node
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/objectstore-triggers.gen.go -package=triggers -tables=object_store_metadata_path,object_store_drain_info
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/secret-triggers.gen.go -package=triggers -tables=secret_backend_rotation,model_secret_backend
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/model-triggers.gen.go -package=triggers -tables=model
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/model-authorized-keys-triggers.gen.go -package=triggers -tables=model_authorized_keys
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/user-authentication-triggers.gen.go -package=triggers -tables=user_authentication

//go:embed controller/sql/*.sql
var controllerSchemaDir embed.FS

const (
	tableExternalController tableNamespaceID = iota + reservedCustomNamespaceIDOffset
	tableControllerNode
	tableControllerConfig
	tableModelMigrationStatus
	tableModelMigrationMinionSync
	tableUpgradeInfo
	tableCloud
	tableCloudCACert
	tableCloudCredential
	tableCloudCredentialAttribute
	tableUpgradeInfoControllerNode
	tableObjectStoreMetadata
	tableObjectStoreDrainInfo
	tableSecretBackendRotation
	tableModelSecretBackend
	tableModelMetadata
	tableModelAuthorizedKeys
	tableUserAuthentication
)

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() *schema.Schema {
	entries, err := controllerSchemaDir.ReadDir("controller/sql")
	if err != nil {
		panic(err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}

	sort.Slice(names, func(i, j int) bool {
		return names[i] < names[j]
	})

	patches := make([]func() schema.Patch, len(names))
	for i, name := range names {
		data, err := controllerSchemaDir.ReadFile(fmt.Sprintf("controller/sql/%s", name))
		if err != nil {
			panic(err)
		}

		patches[i] = func() schema.Patch {
			return schema.MakePatch(string(data))
		}
	}

	// Changestream triggers.
	patches = append(patches,
		triggers.ChangeLogTriggersForCloud("uuid", tableCloud),
		triggers.ChangeLogTriggersForCloudCaCert("cloud_uuid", tableCloudCACert),
		triggers.ChangeLogTriggersForCloudCredential("uuid", tableCloudCredential),
		triggers.ChangeLogTriggersForCloudCredentialAttribute("cloud_credential_uuid", tableCloudCredentialAttribute),
		triggers.ChangeLogTriggersForExternalController("uuid", tableExternalController),
		triggers.ChangeLogTriggersForControllerConfig("key", tableControllerConfig),
		triggers.ChangeLogTriggersForControllerNode("controller_id", tableControllerNode),
		triggers.ChangeLogTriggersForModelMigrationStatus("uuid", tableModelMigrationStatus),
		triggers.ChangeLogTriggersForModelMigrationMinionSync("uuid", tableModelMigrationMinionSync),
		triggers.ChangeLogTriggersForUpgradeInfo("uuid", tableUpgradeInfo),
		triggers.ChangeLogTriggersForUpgradeInfoControllerNode("upgrade_info_uuid", tableUpgradeInfoControllerNode),
		triggers.ChangeLogTriggersForObjectStoreMetadataPath("path", tableObjectStoreMetadata),
		triggers.ChangeLogTriggersForObjectStoreDrainInfo("uuid", tableObjectStoreDrainInfo),
		triggers.ChangeLogTriggersForSecretBackendRotation("backend_uuid", tableSecretBackendRotation),
		triggers.ChangeLogTriggersForModelSecretBackend("model_uuid", tableModelSecretBackend),
		triggers.ChangeLogTriggersForModel("uuid", tableModelMetadata),
		triggers.ChangeLogTriggersForModelAuthorizedKeys("model_uuid", tableModelAuthorizedKeys),
		triggers.ChangeLogTriggersForUserAuthentication("user_uuid", tableUserAuthentication),
	)

	// Generic triggers.
	patches = append(patches,
		// We need to ensure that the internal and kubernetes backends are immutable after
		// they are created by the controller during bootstrap time.
		// 0 is 'controller', 1 is 'kubernetes'.
		triggersForImmutableTable("secret_backend",
			"OLD.backend_type_id IN (0, 1)",
			"secret backends with type controller or kubernetes are immutable"),
	)

	ctrlSchema := schema.New()
	for _, fn := range patches {
		ctrlSchema.Add(fn())
	}

	return ctrlSchema
}
