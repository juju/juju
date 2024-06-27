// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"embed"
	"fmt"
	"sort"

	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/domain/schema/model/triggers"
)

//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/storage-triggers.gen.go -package=triggers -tables=block_device,storage_attachment,storage_filesystem,storage_filesystem_attachment,storage_volume,storage_volume_attachment,storage_volume_attachment_plan
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/model-triggers.gen.go -package=triggers -tables=model_config
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/objectstore-triggers.gen.go -package=triggers -tables=object_store_metadata_path
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/secret-triggers.gen.go -package=triggers -tables=secret_metadata,secret_rotation,secret_revision,secret_revision_expire,secret_revision_obsolete,secret_revision,secret_reference
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/network-triggers.gen.go -package=triggers -tables=subnet
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/machine-triggers.gen.go -package=triggers -tables=machine
//go:generate go run ./../../generate/triggergen -db=model -destination=./model/triggers/user-public-ssh-key.gen.go -package=triggers -tables=user_public_ssh_key

//go:embed model/sql/*.sql
var modelSchemaDir embed.FS

const (
	tableModelConfig tableNamespaceID = iota
	tableModelObjectStoreMetadata
	tableBlockDeviceMachine
	tableStorageAttachment
	tableFileSystem
	tableFileSystemAttachment
	tableVolume
	tableVolumeAttachment
	tableVolumeAttachmentPlan
	tableSecretMetadataAutoPrune
	tableSecretRotation
	tableSecretRevisionObsolete
	tableSecretRevisionExpire
	tableSecretRevision
	tableSecretReference
	tableSubnet
	tableMachine
	tableUserPublicSSHKeys
)

// ModelDDL is used to create model databases.
func ModelDDL() *schema.Schema {
	entries, err := modelSchemaDir.ReadDir("model/sql")
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
		data, err := modelSchemaDir.ReadFile(fmt.Sprintf("model/sql/%s", name))
		if err != nil {
			panic(err)
		}

		patches[i] = func() schema.Patch {
			return schema.MakePatch(string(data))
		}
	}

	// Changestream triggers.
	patches = append(patches,
		triggers.ChangeLogTriggersForBlockDevice("machine_uuid", tableBlockDeviceMachine),
		triggers.ChangeLogTriggersForModelConfig("key", tableModelConfig),
		triggers.ChangeLogTriggersForObjectStoreMetadataPath("path", tableModelObjectStoreMetadata),
		triggers.ChangeLogTriggersForStorageAttachment("storage_instance_uuid", tableStorageAttachment),
		triggers.ChangeLogTriggersForStorageFilesystem("uuid", tableFileSystem),
		triggers.ChangeLogTriggersForStorageFilesystemAttachment("uuid", tableFileSystemAttachment),
		triggers.ChangeLogTriggersForStorageVolume("uuid", tableVolume),
		triggers.ChangeLogTriggersForStorageVolumeAttachment("uuid", tableVolumeAttachment),
		triggers.ChangeLogTriggersForStorageVolumeAttachmentPlan("uuid", tableVolumeAttachmentPlan),
		triggers.ChangeLogTriggersForSecretMetadata("secret_id", tableSecretMetadataAutoPrune),
		triggers.ChangeLogTriggersForSecretRotation("secret_id", tableSecretRotation),
		triggers.ChangeLogTriggersForSecretRevisionObsolete("revision_uuid", tableSecretRevisionObsolete),
		triggers.ChangeLogTriggersForSecretRevisionExpire("revision_uuid", tableSecretRevisionExpire),
		triggers.ChangeLogTriggersForSecretRevision("uuid", tableSecretRevision),
		triggers.ChangeLogTriggersForSecretReference("secret_id", tableSecretReference),
		triggers.ChangeLogTriggersForSubnet("uuid", tableSubnet),
		triggers.ChangeLogTriggersForMachine("machine_id", tableMachine),
		triggers.ChangeLogTriggersForUserPublicSshKey("id", tableUserPublicSSHKeys),
	)

	// Generic triggers.
	patches = append(patches,
		triggersForImmutableTable("model", "", "model table is immutable"),

		// Secret permissions do not allow subject or scope to be updated.
		triggersForImmutableTableUpdates("secret_permission",
			"OLD.subject_type_id <> NEW.subject_type_id OR OLD.scope_uuid <> NEW.scope_uuid OR OLD.scope_type_id <> NEW.scope_type_id",
			"secret permission subjects and scopes are immutable"),
	)

	modelSchema := schema.New()
	for _, fn := range patches {
		modelSchema.Add(fn())
	}
	return modelSchema
}
