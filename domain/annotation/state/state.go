// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

type Annotation struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

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
func (st *State) GetAnnotations(ctx context.Context, ID annotations.ID) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	kind := ID.Kind

	// Prepare query for getting the UUID of ID
	// No need if kind is model, because we keep annotations per model in the DB.
	var kindQueryStmt *sqlair.Statement
	var kindQuery string
	var kindQueryParam sqlair.M

	if kind != annotations.KindModel {
		kindQuery, kindQueryParam, err = uuidQueryForID(ID)
		if err != nil {
			return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", ID.Name)
		}
		kindQueryStmt, err = sqlair.Prepare(kindQuery, sqlair.M{})
		if err != nil {
			return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", ID.Name)
		}
	}

	// Prepare query for getting the annotations of ID
	var getAnnotationsStmt *sqlair.Statement
	if kind == annotations.KindModel {
		getAnnotationsStmt, err = sqlair.Prepare(`SELECT (key, value) AS (&Annotation.*) from annotation_model`, Annotation{}, sqlair.M{})
	} else {
		getAnnotationsQuery := fmt.Sprintf(`
SELECT (key, value) AS (&Annotation.*)
FROM annotation_%[1]s
WHERE %[1]s_uuid = $M.uuid
`, kind)
		getAnnotationsStmt, err = sqlair.Prepare(getAnnotationsQuery, Annotation{}, sqlair.M{})
	}
	if err != nil {
		return nil, errors.Annotatef(err, "preparing get annotations query for ID: %q", ID.Name)
	}

	// Running transactions for getting annotations
	var annotationsResults []Annotation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// If it's for a model, go ahead and run the query (no parameters needed)
		if kind == annotations.KindModel {
			return tx.Query(ctx, getAnnotationsStmt).GetAll(&annotationsResults)
		}
		// Looking up the UUID for ID
		result := sqlair.M{}
		err = tx.Query(ctx, kindQueryStmt, kindQueryParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for ID: %s", ID.Name)
		}

		if len(result) == 0 {
			return errors.Annotatef(err, "unable to find UUID for ID: %q", ID.Name)
		}
		uuid := result["uuid"].(string)

		// Querying for annotations
		return tx.Query(ctx, getAnnotationsStmt, sqlair.M{
			"uuid": uuid,
		}).GetAll(&annotationsResults)
	})

	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			// No errors, we return empty map if no annotation is found
			return nil, nil
		}
		return nil, errors.Annotatef(err, "loading annotations for ID: %q", ID.Name)
	}

	annotations := make(map[string]string, len(annotationsResults))

	for _, result := range annotationsResults {
		annotations[result.Key] = result.Value
	}

	return annotations, errors.Trace(err)
}

// SetAnnotations will add
func (st *State) SetAnnotations(ctx context.Context, ID annotations.ID,
	annotations map[string]string) error {
	return nil
}

// uuidQueryForID is a helper that generates a query and parameters for getting the uuid for a given ID
// We keep different fields to reference different IDs in separate tables, as follows:
// machine: TABLE machine, reference field: machine_id
// unit: TABLE unit, reference field: unit_id
// application: TABLE application, reference field: name
// storage_instance: TABLE storage_instance, reference field: name
// charm: TABLE charm, reference field: url
func uuidQueryForID(ID annotations.ID) (string, sqlair.M, error) {
	kind := ID.Kind
	name := ID.Name

	var query string

	if kind == "machine" || kind == "unit" {
		// Use field <entity>_id (e.g. unit_id) for machines and units
		query = fmt.Sprintf(`SELECT &M.uuid FROM %[1]s WHERE %[1]s_id = $M.entity_id`, kind)
	} else if kind == "application" || kind == "storage_instance" {
		// Use field name for application and storage_instance
		query = fmt.Sprintf(`SELECT &M.uuid FROM %s WHERE name = $M.entity_id`, kind)
	} else if kind == "charm" {
		// Use field url for charm
		query = fmt.Sprintf(`SELECT &M.uuid FROM %s WHERE url = $M.entity_id`, kind)
	} else {
		return "", nil, errors.Errorf("unable to produce query for ID: %q, unknown kind: %q", name, kind)
	}
	return query, sqlair.M{"entity_id": name}, nil
}
