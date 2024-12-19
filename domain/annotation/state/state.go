// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/annotation"
	annotationerrors "github.com/juju/juju/domain/annotation/errors"
	"github.com/juju/juju/internal/errors"
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

// GetAnnotations will retrieve all the annotations associated with the given ID
// from the database.
// If no annotations are found, an empty map is returned.
func (st *State) GetAnnotations(ctx context.Context, id annotations.ID) (map[string]string, error) {

	if id.Kind == annotations.KindModel {
		return st.getAnnotationsForModel(ctx, id)
	}
	return st.getAnnotationsForID(ctx, id)
}

// GetCharmAnnotations will retrieve all the annotations associated with the
// given ID from the database.
// If no annotations are found, an empty map is returned.
func (st *State) GetCharmAnnotations(ctx context.Context, id annotation.GetCharmArgs) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	args := charmArgs{
		Name:     id.Name,
		Revision: id.Revision,
	}

	var results []Annotation
	query, err := st.Prepare(`
SELECT (key, value) AS (&Annotation.*)
FROM annotation_charm AS ac
JOIN v_charm_annotation_index AS c ON ac.uuid = c.uuid
WHERE c.name = $charmArgs.name AND c.revision = $charmArgs.revision;
`, args, Annotation{})
	if err != nil {
		return nil, errors.Errorf("preparing get charm annotations query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, args).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("loading annotations for charm %q: %w", id.Name, err)
	}

	annotations := transform.SliceToMap(results, func(a Annotation) (string, string) {
		return a.Key, a.Value
	})

	return annotations, nil
}

// getAnnotationsForModel retrieves all annotations associated with the given
// model ID from the database.
// If no annotations are found, an empty map is returned.
// This method is specialized to Models as opposed to the other Kinds because we
// keep annotations per model, so we don't need to try to find the UUID of the
// given ID (the model).
func (st *State) getAnnotationsForModel(ctx context.Context, id annotations.ID) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	getAnnotationsStmt, err := st.Prepare(`
SELECT (key, value) AS (&Annotation.*) 
FROM   annotation_model`, Annotation{})
	if err != nil {
		return nil, errors.Errorf("preparing get annotations query for ID: %q: %w", id.Name, err)
	}

	var annotationsResults []Annotation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getAnnotationsStmt).GetAll(&annotationsResults)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("loading annotations for ID: %q: %w", id.Name, err)
	}

	annotations := transform.SliceToMap(annotationsResults, func(a Annotation) (string, string) { return a.Key, a.Value })

	return annotations, nil
}

// getAnnotationsForID retrieves all annotations associated with the given id,
// except IDs of kind ModelKind, from the database.
// If no annotations are found, an empty map is returned.
// This is separate from the getAnnotationsForModel because for non-model ID
// Kinds we need to find the UUID of the ID before we retrieve annotations from
// the corresponding annotation table.
func (st *State) getAnnotationsForID(ctx context.Context, id annotations.ID) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	kindName, err := kindNameFromID(id)
	if err != nil {
		return nil, errors.Capture(err)
	}
	getAnnotationsQuery := fmt.Sprintf(`
SELECT (key, value) AS (&Annotation.*)
FROM annotation_%s
WHERE uuid = $annotationUUID.uuid`, kindName)

	getAnnotationsStmt, err := st.Prepare(getAnnotationsQuery, Annotation{}, annotationUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing get annotations query for ID: %q: %w", id.Name, err)
	}

	kindQuery, kindQueryParam, err := uuidQueryForID(id)
	if err != nil {
		return nil, errors.Errorf("preparing get annotations query for ID: %q: %w", id.Name, err)
	}
	annotationUUIDParam := annotationUUID{UUID: id.String()}
	kindQueryStmt, err := st.Prepare(kindQuery, kindQueryParam, annotationUUIDParam)
	if err != nil {
		return nil, errors.Errorf("preparing get annotations query for ID: %q: %w", id.Name, err)
	}

	var annotationsResults []Annotation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(&annotationUUIDParam)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unable to find UUID for ID: %q %w", id.Name, annotationerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf("looking up UUID for ID: %s: %w", id.Name, err)
		}

		err = tx.Query(ctx, getAnnotationsStmt, annotationUUIDParam).GetAll(&annotationsResults)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("loading annotations for ID: %q: %w", id.Name, err)
	}

	annotations := transform.SliceToMap(annotationsResults, func(a Annotation) (string, string) {
		return a.Key, a.Value
	})

	return annotations, errors.Capture(err)
}

// SetAnnotations associates key/value annotation pairs with a given ID.
// If an annotation already exists for the given ID, then it will be updated
// with the given value. First all annotations are deleted, then the given pairs
// are inserted, so unsetting an annotation is implicit.
func (st *State) SetAnnotations(
	ctx context.Context,
	id annotations.ID,
	values map[string]string,
) error {
	if id.Kind == annotations.KindModel {
		return st.setAnnotationsForModel(ctx, id, values)
	}
	return st.setAnnotationsForID(ctx, id, values)

}

// setAnnotationsForID associates key/value pairs with the given ID.
// This is separate from the setAnnotationsForModel because for non-model ID
// Kinds we need to find the uuid of the id before we add an annotation in the
// corresponding annotation table.
func (st *State) setAnnotationsForID(ctx context.Context, id annotations.ID,
	toInsert map[string]string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	kindName, err := kindNameFromID(id)
	if err != nil {
		return errors.Capture(err)
	}
	insertQuery := fmt.Sprintf(`
INSERT INTO annotation_%s (uuid, key, value)
VALUES ($annotationUUID.uuid, $Annotation.key, $Annotation.value)
	ON CONFLICT(uuid, key) DO UPDATE SET value=$Annotation.value`, kindName)

	setAnnotationsStmt, err := st.Prepare(insertQuery, Annotation{}, annotationUUID{})
	if err != nil {
		return errors.Errorf("preparing set annotations query for ID: %q: %w", id.Name, err)
	}

	deleteQuery := fmt.Sprintf(`
DELETE FROM annotation_%s
WHERE uuid = $annotationUUID.uuid`, kindName)

	deleteAnnotationsStmt, err := st.Prepare(deleteQuery, annotationUUID{})
	if err != nil {
		return errors.Errorf("preparing set annotations query for ID: %q: %w", id.Name, err)
	}

	kindQuery, kindQueryParam, err := uuidQueryForID(id)
	if err != nil {
		return errors.Errorf("preparing uuid retrieval query for ID: %q: %w", id.Name, err)
	}

	var annotationUUID annotationUUID
	kindQueryStmt, err := st.Prepare(kindQuery, annotationUUID, kindQueryParam)
	if err != nil {
		return errors.Errorf("preparing uuid retrieval query for ID: %q: %w", id.Name, err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(&annotationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unable to find UUID for ID: %q %w", id.Name, annotationerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("looking up UUID for ID: %s: %w", id.Name, err)
		}

		if err := tx.Query(ctx, deleteAnnotationsStmt, annotationUUID).Run(); err != nil {
			return errors.Errorf("unsetting annotations for ID: %s: %w", id.Name, err)
		}

		var annotationParam Annotation
		for key, value := range toInsert {
			annotationParam.Key = key
			annotationParam.Value = value
			if err := tx.Query(ctx, setAnnotationsStmt, annotationUUID, annotationParam).Run(); err != nil {
				return errors.Errorf("setting annotations for ID: %s: %w", id.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("setting annotations for ID: %q: %w", id.Name, err)
	}
	return nil
}

// setAnnotationsForModel associates key/value annotation pairs with the model
// referred by the given ID.
// This is specialized to models as opposed to the other Kinds because we keep
// annotations per model, so we don't need to try to find the uuid of the given
// id (the model).
func (st *State) setAnnotationsForModel(ctx context.Context, id annotations.ID,
	toInsert map[string]string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	setAnnotationsStmt, err := st.Prepare(`
INSERT INTO annotation_model (key, value)
VALUES ($Annotation.*)
	ON CONFLICT(key) DO UPDATE SET value=$Annotation.value`, Annotation{})
	if err != nil {
		return errors.Errorf("preparing set annotations query for ID: %q: %w", id.Name, err)
	}
	deleteAnnotationsStmt, err := st.Prepare(`
DELETE FROM annotation_model`)
	if err != nil {
		return errors.Errorf("preparing set annotations query for ID: %q: %w", id.Name, err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteAnnotationsStmt).Run(); err != nil {
			return errors.Errorf("unsetting annotations for ID: %s: %w", id.Name, err)
		}

		var annotationParam Annotation
		for key, value := range toInsert {
			annotationParam.Key = key
			annotationParam.Value = value
			if err := tx.Query(ctx, setAnnotationsStmt, annotationParam).Run(); err != nil {
				return errors.Errorf("setting annotations for ID: %s: %w", id.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("setting model annotations with uuid: %q: %w", id.Name, err)
	}
	return nil
}

// SetCharmAnnotations associates key/value annotation pairs with a given ID.
// If an annotation already exists for the given ID, then it will be updated
// with the given value. First all annotations are deleted, then the given pairs
// are inserted, so unsetting an annotation is implicit.
func (st *State) SetCharmAnnotations(
	ctx context.Context,
	id annotation.GetCharmArgs,
	values map[string]string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	args := charmArgs{
		Name:     id.Name,
		Revision: id.Revision,
	}

	getStmt, err := st.Prepare(`
SELECT &annotationUUID.* FROM v_charm_annotation_index
WHERE name = $charmArgs.name AND revision = $charmArgs.revision;
`, args, annotationUUID{})
	if err != nil {
		return errors.Errorf("preparing get charm annotations query: %w", err)
	}

	deleteStmt, err := st.Prepare(`
DELETE FROM annotation_charm
WHERE uuid = $annotationUUID.uuid
`, annotationUUID{})
	if err != nil {
		return errors.Errorf("preparing delete charm annotations query: %w", err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO annotation_charm (*)
VALUES ($annotationUUID.*, $Annotation.*)
`, Annotation{}, annotationUUID{})
	if err != nil {
		return errors.Errorf("preparing set charm annotations query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var annotationUUID annotationUUID
		err = tx.Query(ctx, getStmt, args).Get(&annotationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unable to find UUID for ID: %q %w", id.Name, annotationerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("looking up UUID for ID: %s: %w", id.Name, err)
		}

		if err := tx.Query(ctx, deleteStmt, annotationUUID).Run(); err != nil {
			return errors.Errorf("unsetting annotations for ID: %s: %w", id.Name, err)
		}

		var annotationParam Annotation
		for key, value := range values {
			annotationParam.Key = key
			annotationParam.Value = value
			if err := tx.Query(ctx, insertStmt, annotationUUID, annotationParam).Run(); err != nil {
				return errors.Errorf("setting annotations for ID: %s: %w", id.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("setting charm annotations for ID: %q: %w", id.Name, err)
	}
	return nil
}

// uuidQueryForID generates a query and parameters for getting the uuid for a
// given annotation ID.
//
// We keep different fields to reference different IDs in separate tables, as
// follows:
//
//	machine: TABLE machine, reference field: name
//	unit: TABLE unit, reference field: name
//	application: TABLE application, reference field: name
func uuidQueryForID(id annotations.ID) (string, sqlair.M, error) {
	kindName, err := kindNameFromID(id)
	if err != nil {
		return "", sqlair.M{}, errors.Capture(err)
	}

	var selector string
	switch id.Kind {
	case annotations.KindMachine:
		selector = "name"
	case annotations.KindUnit:
		selector = "name"
	case annotations.KindApplication:
		selector = "name"
	case annotations.KindStorage:
		selector = "name"
	}

	query := fmt.Sprintf(`SELECT &annotationUUID.uuid FROM %s WHERE %s = $M.entity_id`, kindName, selector)
	return query, sqlair.M{"entity_id": id.Name}, nil
}

// kindNameFromID keeps the field names that's used for different ID.Kinds in
// the database. Used in deducing the table name (e.g. annotation_<ID.Kind>),
// as well as fields like <ID.Kind>_uuid in the corresponding table.
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
	case annotations.KindModel:
		kindName = "model"
	default:
		return "", errors.Errorf("%q: %w", id.Kind, annotationerrors.UnknownKind)
	}
	return kindName, nil
}
