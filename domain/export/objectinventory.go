// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"github.com/juju/juju/core/database"
	ctrlv4_1_0 "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/internal/errors"
)

// ObjectInventoryEntry identifies one object referenced by a database
// export. Objects are keyed by (Namespace, SHA384): multiple logical
// paths referring to one object need a single entry per namespace,
// while identical content in different namespaces requires one entry
// per namespace.
type ObjectInventoryEntry struct {
	// Namespace is the object store namespace owning the object: the
	// literal "controller" for controller database objects, or a model
	// UUID.
	Namespace string

	// SHA256 is the full SHA-256 hash of the object's content.
	SHA256 string

	// SHA384 is the full SHA-384 hash of the object's content, used as
	// the object's file name in the object store and in backup
	// archives.
	SHA384 string

	// Size is the object's content size in bytes.
	Size int64
}

// ControllerObjectInventory extracts the object inventory referenced by
// a controller export, namespaced to [database.ControllerNS].
// One entry is produced per distinct SHA-384.
func ControllerObjectInventory(export ControllerExport) ([]ObjectInventoryEntry, error) {
	payload, ok := export.Payload.(*ctrlv4_1_0.ControllerExport)
	if !ok {
		return nil, errors.Errorf(
			"unexpected controller export payload type %T", export.Payload)
	}

	rows := payload.ObjectStoreMetadata
	entries := make([]ObjectInventoryEntry, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.Sha384]; ok {
			continue
		}
		seen[row.Sha384] = struct{}{}
		entries = append(entries, ObjectInventoryEntry{
			Namespace: database.ControllerNS,
			SHA256:    row.Sha256,
			SHA384:    row.Sha384,
			Size:      row.Size,
		})
	}
	return entries, nil
}

// ModelObjectInventory extracts the object inventory referenced by a
// model export, namespaced to the given model UUID. One entry is
// produced per distinct SHA-384.
func ModelObjectInventory(export ModelExport, modelUUID string) ([]ObjectInventoryEntry, error) {
	payload, ok := export.Payload.(*latest.ModelExport)
	if !ok {
		return nil, errors.Errorf(
			"unexpected model export payload type %T", export.Payload)
	}

	rows := payload.ObjectStoreMetadata
	entries := make([]ObjectInventoryEntry, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.Sha384]; ok {
			continue
		}
		seen[row.Sha384] = struct{}{}
		entries = append(entries, ObjectInventoryEntry{
			Namespace: modelUUID,
			SHA256:    row.Sha256,
			SHA384:    row.Sha384,
			Size:      row.Size,
		})
	}
	return entries, nil
}
