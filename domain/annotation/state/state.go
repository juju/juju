// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	annotationerrors "github.com/juju/juju/domain/annotation/errors"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetAnnotations will retrieve all the annotations associated with the given ID from the database.
// If no annotations are found, an empty map is returned.
func (st *State) GetAnnotations(ctx context.Context, id annotations.ID) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	kindName, err := kindNameFromID(id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Prepare query for getting the UUID of ID
	// No need if kind is model, because we keep annotations per model in the DB.
	var kindQueryStmt *sqlair.Statement
	var kindQuery string
	var kindQueryParam sqlair.M

	if id.Kind != annotations.KindModel {
		kindQuery, kindQueryParam, err = uuidQueryForID(id)
		if err != nil {
			return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
		}
		kindQueryStmt, err = sqlair.Prepare(kindQuery, sqlair.M{})
		if err != nil {
			return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
		}
	}

	// Prepare query for getting the annotations of ID
	var getAnnotationsQuery string
	var getAnnotationsStmt *sqlair.Statement

	if id.Kind == annotations.KindModel {
		getAnnotationsQuery = `SELECT (key, value) AS (&Annotation.*) from annotation_model`
	} else {
		getAnnotationsQuery = fmt.Sprintf(`
SELECT (key, value) AS (&Annotation.*)
FROM annotation_%[1]s
WHERE %[1]s_uuid = $M.uuid
`, kindName)
	}

	getAnnotationsStmt, err = sqlair.Prepare(getAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
	}

	// Running transactions for getting annotations
	var annotationsResults []Annotation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// If it's for a model, go ahead and run the query (no uuid needed)
		if id.Kind == annotations.KindModel {
			return tx.Query(ctx, getAnnotationsStmt).GetAll(&annotationsResults)
		}
		// Looking up the UUID for ID
		result := sqlair.M{}
		err = tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for ID: %s", id.Name)
		}

		if len(result) == 0 {
			return fmt.Errorf("unable to find UUID for ID: %q %w", id.Name, errors.NotFound)
		}

		uuid, ok := result["uuid"].(string)
		if !ok {
			return fmt.Errorf("unable to find UUID for ID: %q %w", id.Name, errors.NotFound)
		}
		// Querying for annotations
		return tx.Query(ctx, getAnnotationsStmt, sqlair.M{
			"uuid": uuid,
		}).GetAll(&annotationsResults)
	})

	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			// No errors, we return empty map if no annotation is found
			return map[string]string{}, nil
		}
		return nil, errors.Annotatef(err, "loading annotations for ID: %q", id.Name)
	}

	annotations := make(map[string]string, len(annotationsResults))

	for _, result := range annotationsResults {
		annotations[result.Key] = result.Value
	}

	return annotations, errors.Trace(err)
}

// SetAnnotations associates key/value annotation pairs with a given ID.
// If annotation already exists for the given ID, then it will be updated with
// the given value. Setting a key's value to "" will remove the key from the annotations map
// (functionally unsetting the key).
func (st *State) SetAnnotations(ctx context.Context, id annotations.ID,
	annotationsParam map[string]string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Separate the annotations that are to be setted vs removed
	toUpsert := make(map[string]string)
	var toRemove []string

	for key, value := range annotationsParam {
		if value == "" {
			toRemove = append(toRemove, fmt.Sprintf("'%s'", key))
		} else {
			toUpsert[key] = value
		}
	}

	if id.Kind == annotations.KindModel {
		return st.setAnnotationsForModel(ctx, db, id, toUpsert, toRemove)
	}
	return st.setAnnotationsForID(ctx, db, id, toUpsert, toRemove)
}

// setAnnotationsForID associates key/value pairs with the given ID. This is separate from the
// setAnnotationsForModel because for non-model ID Kinds we need to find the uuid of the id before
// we add an annotation in the corresponding annotation table.
func (st *State) setAnnotationsForID(ctx context.Context, db database.TxnRunner, id annotations.ID,
	toUpsert map[string]string, toRemove []string) error {
	// extract kindName to use in query generation for kind-specific fields and table names
	kindName, err := kindNameFromID(id)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for getting the UUID of id.
	kindQuery, kindQueryParam, err := uuidQueryForID(id)
	if err != nil {
		return errors.Annotatef(err, "preparing uuid retrieval query for ID: %q", id.Name)
	}
	kindQueryStmt, err := sqlair.Prepare(kindQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing uuid retrieval query for ID: %q", id.Name)
	}

	// Prepare query for inserting and deleting annotations for id
	var setAnnotationsQuery string
	var deleteAnnotationsQuery string

	var setAnnotationsStmt *sqlair.Statement
	var deleteAnnotationsStmt *sqlair.Statement

	// Set query
	setAnnotationsQuery = fmt.Sprintf(`
INSERT INTO annotation_%[1]s (%[1]s_uuid, key, value)
VALUES ($M.uuid, $M.key, $M.value)
ON CONFLICT(%[1]s_uuid, key) DO UPDATE SET value=$M.value`, kindName)
	// Delete query
	deleteAnnotationsQuery = fmt.Sprintf(`
DELETE FROM annotation_%[1]s
WHERE %[1]s_uuid = $M.uuid AND key IN (%[2]s)`, kindName, strings.Join(toRemove, ", "))

	// Prepare sqlair statements
	setAnnotationsStmt, err = sqlair.Prepare(setAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing set annotations query for ID: %q", id.Name)
	}
	deleteAnnotationsStmt, err = sqlair.Prepare(deleteAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing set annotations query for ID: %q", id.Name)
	}

	// Running transactions using sqlair statements
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We need to find the uuid of ID first, so looking it up
		result := sqlair.M{}
		err = tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for ID: %s", id.Name)
		}

		if len(result) == 0 {
			return fmt.Errorf("unable to find UUID for ID: %q %w", id.Name, errors.NotFound)
		}
		uuid, ok := result["uuid"].(string)
		if !ok {
			return fmt.Errorf("unable to find UUID for ID: %q %w", id.Name, errors.NotFound)
		}

		// Unset the annotations
		if err := tx.Query(ctx, deleteAnnotationsStmt, sqlair.M{
			"uuid": uuid,
		}).Run(); err != nil {
			return errors.Annotatef(err, "unsetting annotations for ID: %s", id.Name)
		}

		// Insert annotations
		for key, value := range toUpsert {
			if err := tx.Query(ctx, setAnnotationsStmt, sqlair.M{
				"uuid":  uuid,
				"key":   key,
				"value": value,
			}).Run(); err != nil {
				return errors.Annotatef(err, "setting annotations for ID: %s", id.Name)
			}
		}
		return nil
	})

	if err != nil {
		return errors.Annotatef(err, "setting annotations for ID: %q", id.Name)
	}

	return nil
}

// setAnnotationsForModel associates key/value annotation pairs with the model referred by the given
// ID. This is specialized as opposed to the other Kinds because we keep annotations per model, so
// we don't need to try to find the uuid of the given id (the model).
func (st *State) setAnnotationsForModel(ctx context.Context, db database.TxnRunner, id annotations.ID,
	toUpsert map[string]string, toRemove []string) error {
	var setAnnotationsQuery string
	var deleteAnnotationsQuery string

	var setAnnotationsStmt *sqlair.Statement
	var deleteAnnotationsStmt *sqlair.Statement

	// Set query
	setAnnotationsQuery = `
INSERT INTO annotation_model (key, value)
VALUES ($M.key, $M.value)
	ON CONFLICT(key) DO UPDATE SET value=$M.value`

	// Delete query
	deleteAnnotationsQuery = fmt.Sprintf(`DELETE FROM annotation_model WHERE key IN (%s)`, strings.Join(toRemove, ", "))

	// Prepare sqlair statements
	setAnnotationsStmt, err := sqlair.Prepare(setAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
	}
	deleteAnnotationsStmt, err = sqlair.Prepare(deleteAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
	}

	// Running transactions using sqlair statements
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Unset the annotations
		if err := tx.Query(ctx, deleteAnnotationsStmt).Run(); err != nil {
			return errors.Annotatef(err, "unsetting annotations for ID: %s", id.Name)
		}

		// Insert annotations
		for key, value := range toUpsert {
			if err := tx.Query(ctx, setAnnotationsStmt, sqlair.M{
				"key":   key,
				"value": value,
			}).Run(); err != nil {
				return errors.Annotatef(err, "setting annotations for ID: %s", id.Name)
			}
		}
		return nil
	})

	if err != nil {
		return errors.Annotatef(err, "setting annotations for model with uuid: %q", id.Name)
	}

	return nil
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
