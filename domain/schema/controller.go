// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"embed"

	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/core/semversion"
	coreversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/schema/controller/triggers"
)

//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/cloud-triggers.gen.go -package=triggers -tables=cloud,cloud_ca_cert,cloud_credential,cloud_credential_attribute
//go:generate go run ./../../generate/triggergen -db=controller -destination=./controller/triggers/controller-triggers.gen.go -package=triggers -tables=controller_config,controller_node,external_controller,controller_api_address
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
	tableControllerAPIAddress
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

// controllerPostPatchFilesByVersion is used to categorise the post patch files
// to particular versions of Juju. To include a new post patch file, it must be
// added to the list for the version in which it is first applied.
//
// Also, post-patch files are only applicable for differences in patch versions
// within the same major.minor version. So all entries should be of the same
// major.minor version as the current version. The full version is only included
// for clarity of reading
var controllerPostPatchFilesByVersion = []struct {
	version semversion.Number
	files   []string
}{{
	version: semversion.MustParse("4.0.1"),
	files:   []string{"0026-secret-backend.PATCH.sql"},
}}

// ControllerDDL is used to create the controller database schema at bootstrap.
func ControllerDDL() *schema.Schema {
	return ControllerDDLForVersion(coreversion.Current)
}

// ControllerDDLForVersion returns the controller database schema for the
// specified version. The version must match the current major.minor version
func ControllerDDLForVersion(version semversion.Number) *schema.Schema {
	if version.Major != coreversion.Current.Major || version.Minor != coreversion.Current.Minor {
		panic("Cannot return the controller DDL for a different major.minor version")
	}

	patches, err := readPatches(controllerSchemaDir, "controller/sql")
	if err != nil {
		panic(err)
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
		triggers.ChangeLogTriggersForControllerApiAddress("controller_id", tableControllerAPIAddress),
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

	var postPatchFiles []string
	for _, postPatch := range controllerPostPatchFilesByVersion {
		if postPatch.version.Compare(version) <= 0 {
			postPatchFiles = append(postPatchFiles, postPatch.files...)
		}
	}
	postPatches, err := readPostPatches(controllerSchemaDir, "controller/sql", postPatchFiles)
	if err != nil {
		panic(err)
	}

	ctrlSchema := schema.New()
	for _, fn := range patches {
		ctrlSchema.Add(fn())
	}

	for _, fn := range postPatches {
		ctrlSchema.Add(fn())
	}

	return ctrlSchema
}
