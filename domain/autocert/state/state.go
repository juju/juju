// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/uuid"
)

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

	uuid, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	autocert := dbAutocert{
		UUID:     uuid.String(),
		Name:     name,
		Data:     string(data),
		Encoding: 0,
	}

	q, err := st.Prepare(`
INSERT INTO autocert_cache (*)
VALUES ($dbAutocert.*)
  ON CONFLICT(name) DO UPDATE SET data=excluded.data`, autocert)
	if err != nil {
		return errors.Annotatef(err, "preparing insert autocert into cache")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, q, autocert).Run(); err != nil {
			return errors.Trace(domain.CoerceError(err))
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

	autocert := dbAutocert{Name: name}

	q := `
SELECT (name, data) AS (&dbAutocert.*)
FROM   autocert_cache 
WHERE  name = $dbAutocert.name`
	s, err := st.Prepare(q, autocert)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing autocert select statement")
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, autocert).Get(&autocert))
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.Annotatef(errors.NotFound, "autocert %s", name)
		}
		return nil, errors.Annotate(domain.CoerceError(err), "querying autocert cache")
	}

	return []byte(autocert.Data), nil
}

// Delete implements autocert.Cache.Delete.
func (st *State) Delete(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	certToDelete := dbAutocert{Name: name}
	stmt, err := st.Prepare(`DELETE FROM autocert_cache WHERE name = $dbAutocert.name`, certToDelete)
	if err != nil {
		return errors.Annotatef(err, "preparing autocert cache delete statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, certToDelete).Run(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
		return nil
	})

	return errors.Trace(err)
}
