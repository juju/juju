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

	getAnnotationsQuery = getAnnotationQueryForID(id)

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
	setAnnotationsQuery := setAnnotationQueryForID(id)
	deleteAnnotationsQuery := deleteAnnotationsQueryForID(id, strings.Join(toRemove, ", "))

	// Prepare sqlair statements
	setAnnotationsStmt, err := sqlair.Prepare(setAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing set annotations query for ID: %q", id.Name)
	}
	deleteAnnotationsStmt, err := sqlair.Prepare(deleteAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing set annotations query for ID: %q", id.Name)
	}

	// Running transactions using sqlair statements
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We need to find the uuid of ID first, so looking it up
		result := sqlair.M{}
		err = tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(&result)
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

	setAnnotationsQuery := setAnnotationQueryForID(id)
	deleteAnnotationsQuery := deleteAnnotationsQueryForID(id, strings.Join(toRemove, ", "))

	// Prepare sqlair statements
	setAnnotationsStmt, err := sqlair.Prepare(setAnnotationsQuery, Annotation{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing get annotations query for ID: %q", id.Name)
	}
	deleteAnnotationsStmt, err := sqlair.Prepare(deleteAnnotationsQuery, Annotation{}, sqlair.M{})
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
