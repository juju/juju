// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// Autocert is a named certificate.
type Autocert struct {
	// Name is the autocert name. It uniquely identifies the certificate.
	Name string `db:"name"`

	// Data represents the binary (encoded) contents of the autocert.
	Data string `db:"data"`
}

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory coreDB.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// Put implements autocert.Cache.Put.
func (st *State) Put(ctx context.Context, name string, data []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
INSERT INTO autocert_cache (uuid, name, data, encoding)
VALUES (?, ?, ?, 0)
  ON CONFLICT(name) DO UPDATE SET data=excluded.data`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, q, uuid.String(), name, string(data)); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	return errors.Trace(err)
}

// Get implements autocert.Cache.Get.
func (st *State) Get(ctx context.Context, name string) ([]byte, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT (name, data) AS (&Autocert.*)
FROM   autocert_cache 
WHERE  name = $M.name`
	s, err := sqlair.Prepare(q, Autocert{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var row Autocert
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"name": name}).Get(&row))
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.Annotatef(errors.NotFound, "autocert %s", name)
		}
		return nil, errors.Annotate(err, "querying autocert cache")
	}

	return []byte(row.Data), nil
}

// Delete implements autocert.Cache.Delete.
func (st *State) Delete(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `DELETE FROM autocert_cache WHERE name = ?`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, q, name); err != nil {
			return errors.Trace(err)
		}

		return nil
	})

	return errors.Trace(err)
}
