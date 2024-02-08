// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/annotations"
	annotationerrors "github.com/juju/juju/domain/annotation/errors"
)

// getAnnotationQueryForID provides a query for the given id, based on pre-computed queries for
// GetAnnotations for different kinds of ids. We keep these static (avoiding dynamically generating
// table names and fields) to keep things safe.
func getAnnotationQueryForID(id annotations.ID) string {
	var query string
	switch id.Kind {
	case annotations.KindModel:
		query = `SELECT (key, value) AS (&Annotation.*) from annotation_model`
	case annotations.KindMachine:
		query = `
SELECT (key, value) AS (&Annotation.*)
FROM annotation_machine
WHERE machine_uuid = $M.uuid`
	case annotations.KindUnit:
		query = `
SELECT (key, value) AS (&Annotation.*)
FROM annotation_unit
WHERE unit_uuid = $M.uuid`
	case annotations.KindApplication:
		query = `
SELECT (key, value) AS (&Annotation.*)
FROM annotation_application
WHERE application_uuid = $M.uuid`
	case annotations.KindStorage:
		query = `
SELECT (key, value) AS (&Annotation.*)
FROM annotation_storage_instance
WHERE storage_instance_uuid = $M.uuid`
	case annotations.KindCharm:
		query = `
SELECT (key, value) AS (&Annotation.*)
FROM annotation_charm
WHERE charm_uuid = $M.uuid`
	}
	return query
}

// setAnnotationQueryForID provides a query for the given id, based on pre-computed queries for
// SetAnnotations for different kinds of ids. We keep these static (avoiding dynamically generating
// table names and fields) to keep things safe.
func setAnnotationQueryForID(id annotations.ID) string {
	var query string
	switch id.Kind {
	case annotations.KindModel:
		query = `
INSERT INTO annotation_model (key, value)
VALUES ($M.key, $M.value)
	ON CONFLICT(key) DO UPDATE SET value=$M.value`
	case annotations.KindMachine:
		query = `
INSERT INTO annotation_machine (machine_uuid, key, value)
VALUES ($M.uuid, $M.key, $M.value)
	ON CONFLICT(machine_uuid, key) DO UPDATE SET value=$M.value`
	case annotations.KindUnit:
		query = `
INSERT INTO annotation_unit (unit_uuid, key, value)
VALUES ($M.uuid, $M.key, $M.value)
	ON CONFLICT(unit_uuid, key) DO UPDATE SET value=$M.value`
	case annotations.KindApplication:
		query = `
INSERT INTO annotation_application (application_uuid, key, value)
VALUES ($M.uuid, $M.key, $M.value)
	ON CONFLICT(application_uuid, key) DO UPDATE SET value=$M.value`
	case annotations.KindStorage:
		query = `
INSERT INTO annotation_storage_instance (storage_instance_uuid, key, value)
VALUES ($M.uuid, $M.key, $M.value)
	ON CONFLICT(storage_instance_uuid, key) DO UPDATE SET value=$M.value`
	case annotations.KindCharm:
		query = `
INSERT INTO annotation_charm (charm_uuid, key, value)
VALUES ($M.uuid, $M.key, $M.value)
	ON CONFLICT(charm_uuid, key) DO UPDATE SET value=$M.value`
	}
	return query
}

// deleteAnnotationQueryForID provides a query for the given id, based on pre-computed queries for
// SetAnnotations for different kinds of ids. We keep these static (avoiding dynamically generating
// table names and fields) to keep things safe.
func deleteAnnotationsQueryForID(id annotations.ID, toRemoveBindings string) string {
	var query string
	switch id.Kind {
	case annotations.KindModel:
		query = fmt.Sprintf(`DELETE FROM annotation_model WHERE key IN (%s)`, toRemoveBindings)
	case annotations.KindMachine:
		query = fmt.Sprintf(`
DELETE FROM annotation_machine
WHERE machine_uuid = $M.uuid AND key IN (%s)`, toRemoveBindings)
	case annotations.KindUnit:
		query = fmt.Sprintf(`
DELETE FROM annotation_unit
WHERE unit_uuid = $M.uuid AND key IN (%s)`, toRemoveBindings)
	case annotations.KindApplication:
		query = fmt.Sprintf(`
DELETE FROM annotation_application
WHERE application_uuid = $M.uuid AND key IN (%s)`, toRemoveBindings)
	case annotations.KindStorage:
		query = fmt.Sprintf(`
DELETE FROM annotation_storage_instance
WHERE storage_instance_uuid = $M.uuid AND key IN (%s)`, toRemoveBindings)
	case annotations.KindCharm:
		query = fmt.Sprintf(`
DELETE FROM annotation_charm
WHERE charm_uuid = $M.uuid AND key IN (%s)`, toRemoveBindings)
	}
	return query
}

// uuidQueryForID generates a query and parameters for getting the uuid for a given ID
// We keep different fields to reference different IDs in separate tables, as follows:
// machine: TABLE machine, reference field: machine_id
// unit: TABLE unit, reference field: unit_id
// application: TABLE application, reference field: name
// storage_instance: TABLE storage_instance, reference field: name
// charm: TABLE charm, reference field: url
func uuidQueryForID(id annotations.ID) (string, sqlair.M, error) {
	kindName, err := kindNameFromID(id)
	if err != nil {
		return "", sqlair.M{}, errors.Trace(err)
	}

	var selector string
	switch id.Kind {
	case annotations.KindMachine:
		selector = "machine_id"
	case annotations.KindUnit:
		selector = "unit_id"
	case annotations.KindApplication:
		selector = "name"
	case annotations.KindStorage:
		selector = "name"
	case annotations.KindCharm:
		// TODO(cderici): This selector is subject to change when charm domain is added.
		selector = "url"
	}

	query := fmt.Sprintf(`SELECT &M.uuid FROM %s WHERE %s = $M.entity_id`, kindName, selector)
	return query, sqlair.M{"entity_id": id.Name}, nil
}

// kindNameFromID keeps the field names that's used for different ID.Kinds in the database. Used in
// deducing the table name (e.g. annotation_<ID.Kind>), as well as fields like <ID.Kind>_uuid in the
// corresponding table.
func kindNameFromID(id annotations.ID) (string, error) {
	var kindName string
	switch id.Kind {
	case annotations.KindMachine:
		kindName = "machine"
	case annotations.KindUnit:
		kindName = "unit"
	case annotations.KindApplication:
		kindName = "application"
	case annotations.KindStorage:
		kindName = "storage_instance"
	case annotations.KindCharm:
		kindName = "charm"
	case annotations.KindModel:
		kindName = "model"
	default:
		return "", errors.Annotatef(annotationerrors.AnnotationUnknownKind, "%q", id.Kind)
	}
	return kindName, nil
}
