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
func getAnnotationQueryForID(id annotations.ID) (string, error) {
	if id.Kind == annotations.KindModel {
		return `SELECT (key, value) AS (&Annotation.*) from annotation_model`, nil
	}

	kindName, err := kindNameFromID(id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf(`
SELECT (key, value) AS (&Annotation.*)
FROM annotation_%s
WHERE uuid = $M.uuid`, kindName), nil
}

// setAnnotationQueryForID provides a query for the given id, based on pre-computed queries for
// SetAnnotations for different kinds of ids. We keep these static (avoiding dynamically generating
// table names and fields) to keep things safe.
func setAnnotationQueryForID(id annotations.ID) (string, error) {
	if id.Kind == annotations.KindModel {
		return `
INSERT INTO annotation_model (key, value)
VALUES ($M.key, $M.value)
	ON CONFLICT(key) DO UPDATE SET value=$M.value`, nil
	}

	kindName, err := kindNameFromID(id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf(`
INSERT INTO annotation_%s (uuid, key, value)
VALUES ($M.uuid, $M.key, $M.value)
	ON CONFLICT(uuid, key) DO UPDATE SET value=$M.value`, kindName), nil
}

// deleteAnnotationQueryForID provides a query for the given id, based on pre-computed queries for
// SetAnnotations for different kinds of ids.
func deleteAnnotationsQueryForID(id annotations.ID) (string, error) {
	if id.Kind == annotations.KindModel {
		return `DELETE FROM annotation_model`, nil
	} else {
		kindName, err := kindNameFromID(id)
		if err != nil {
			return "", errors.Trace(err)
		}
		return fmt.Sprintf(`
DELETE FROM annotation_%s
WHERE uuid = $M.uuid`, kindName), nil
	}
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
		kindName = "v_charm_url"
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
		return "", errors.Annotatef(annotationerrors.UnknownKind, "%q", id.Kind)
	}
	return kindName, nil
}
