// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
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
	getAnnotationsQuery, err := getAnnotationQueryForID(id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	annotationUUIDParam := annotationUUID{}
	getAnnotationsStmt, err := st.Prepare(getAnnotationsQuery, Annotation{}, annotationUUIDParam)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
	}

	if id.Kind == annotations.KindModel {
		return st.getAnnotationsForModel(ctx, id, getAnnotationsStmt)
	}
	return st.getAnnotationsForID(ctx, id, getAnnotationsStmt, annotationUUIDParam)
}

// getAnnotationsForModel retrieves all annotations associated with the given
// model ID from the database.
// If no annotations are found, an empty map is returned.
// This method is specialized to Models as opposed to the other Kinds because we
// keep annotations per model, so we don't need to try to find the UUID of the
// given ID (the model).
func (st *State) getAnnotationsForModel(ctx context.Context, id annotations.ID, getAnnotationsStmt *sqlair.Statement) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
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
		return nil, errors.Annotatef(err, "loading annotations for ID: %q", id.Name)
	}

	annotations := transform.SliceToMap(annotationsResults, func(a Annotation) (string, string) { return a.Key, a.Value })

	return annotations, nil
}

// getAnnotationsForID retrieves all annotations associated with the given id
// from the database.
// If no annotations are found, an empty map is returned.
// This is separate from the getAnnotationsForModel because for non-model ID
// Kinds we need to find the UUID of the ID before we retrieve annotations from
// the corresponding annotation table.
func (st *State) getAnnotationsForID(ctx context.Context, id annotations.ID, getAnnotationsStmt *sqlair.Statement, annotationUUIDParam annotationUUID) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	kindQuery, kindQueryParam, err := uuidQueryForID(id)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
	}
	kindQueryStmt, err := st.Prepare(kindQuery, kindQueryParam, annotationUUIDParam)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
	}

	var annotationsResults []Annotation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(&annotationUUIDParam)
		if errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("unable to find UUID for ID: %q %w", id.Name, errors.NotFound)
		}
		if err != nil {
			return errors.Annotatef(err, "looking up UUID for ID: %s", id.Name)
		}

		err = tx.Query(ctx, getAnnotationsStmt, annotationUUIDParam).GetAll(&annotationsResults)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Annotatef(err, "loading annotations for ID: %q", id.Name)
	}

	annotations := transform.SliceToMap(annotationsResults, func(a Annotation) (string, string) {
		return a.Key, a.Value
	})

	return annotations, errors.Trace(err)
}

// SetAnnotations associates key/value annotation pairs with a given ID.
// If an annotation already exists for the given ID, then it will be updated
// with the given value. First all annotations are deleted, then the given pairs
// are inserted, so unsetting an annotation is implicit.
func (st *State) SetAnnotations(
	ctx context.Context,
	id annotations.ID,
	annotationsParam map[string]string,
) error {
	toInsert := make(map[string]string)

	for key, value := range annotationsParam {
		if strings.Contains(key, ".") {
			return errors.Errorf("invalid key %q", key)
		}
		if value != "" {
			toInsert[key] = value
		}
	}

	setAnnotationsQuery, err := setAnnotationQueryForID(id)
	if err != nil {
		return errors.Trace(err)
	}
	deleteAnnotationsQuery, err := deleteAnnotationsQueryForID(id)
	if err != nil {
		return errors.Trace(err)
	}

	annotationUUIDParam := annotationUUID{}
	annotationParam := Annotation{}
	setAnnotationsStmt, err := st.Prepare(setAnnotationsQuery, annotationParam, annotationUUIDParam)
	if err != nil {
		return errors.Annotatef(err, "preparing set annotations query for ID: %q", id.Name)
	}
	deleteAnnotationsStmt, err := st.Prepare(deleteAnnotationsQuery, annotationUUIDParam)
	if err != nil {
		return errors.Annotatef(err, "preparing set annotations query for ID: %q", id.Name)
	}

	if id.Kind == annotations.KindModel {
		return st.setAnnotationsForModel(ctx, id, toInsert, setAnnotationsStmt, deleteAnnotationsStmt, annotationParam)
	}
	return st.setAnnotationsForID(ctx, id, toInsert,
		setAnnotationsStmt, deleteAnnotationsStmt, annotationUUIDParam, annotationParam)
}

// setAnnotationsForID associates key/value pairs with the given ID.
// This is separate from the setAnnotationsForModel because for non-model ID
// Kinds we need to find the uuid of the id before we add an annotation in the
// corresponding annotation table.
func (st *State) setAnnotationsForID(ctx context.Context, id annotations.ID,
	toInsert map[string]string,
	setAnnotationsStmt *sqlair.Statement,
	deleteAnnotationsStmt *sqlair.Statement,
	annotationUUIDParam annotationUUID,
	annotationParam Annotation,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	kindQuery, kindQueryParam, err := uuidQueryForID(id)
	if err != nil {
		return errors.Annotatef(err, "preparing uuid retrieval query for ID: %q", id.Name)
	}
	kindQueryStmt, err := st.Prepare(kindQuery, annotationUUIDParam, kindQueryParam)
	if err != nil {
		return errors.Annotatef(err, "preparing uuid retrieval query for ID: %q", id.Name)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(&annotationUUIDParam)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("unable to find UUID for ID: %q %w", id.Name, errors.NotFound)
			}
			return errors.Annotatef(err, "looking up UUID for ID: %s", id.Name)
		}

		if err := tx.Query(ctx, deleteAnnotationsStmt, annotationUUIDParam).Run(); err != nil {
			return errors.Annotatef(err, "unsetting annotations for ID: %s", id.Name)
		}

		for key, value := range toInsert {
			annotationParam.Key = key
			annotationParam.Value = value
			if err := tx.Query(ctx, setAnnotationsStmt, annotationUUIDParam, annotationParam).Run(); err != nil {
				return errors.Annotatef(err, "setting annotations for ID: %s", id.Name)
			}
		}
		return nil
	})

	return errors.Annotatef(err, "setting annotations for ID: %q", id.Name)
}

// setAnnotationsForModel associates key/value annotation pairs with the model
// referred by the given ID.
// This is specialized to models as opposed to the other Kinds because we keep
// annotations per model, so we don't need to try to find the uuid of the given
// id (the model).
func (st *State) setAnnotationsForModel(ctx context.Context, id annotations.ID,
	toInsert map[string]string,
	setAnnotationsStmt *sqlair.Statement,
	deleteAnnotationsStmt *sqlair.Statement,
	annotationParam Annotation,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteAnnotationsStmt).Run(); err != nil {
			return errors.Annotatef(err, "unsetting annotations for ID: %s", id.Name)
		}

		for key, value := range toInsert {
			annotationParam.Key = key
			annotationParam.Value = value
			if err := tx.Query(ctx, setAnnotationsStmt, annotationParam).Run(); err != nil {
				return errors.Annotatef(err, "setting annotations for ID: %s", id.Name)
			}
		}
		return nil
	})

	return errors.Annotatef(err, "setting annotations for model with uuid: %q", id.Name)
}
