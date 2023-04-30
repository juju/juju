package modelmanager

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

type ModelManagerState interface {
	Create(ctx context.Context, uuid string) error
}

type DBState struct {
	dbGetter facade.ConterollerDBGetter
}

func NewDBState(dbGetter facade.ConterollerDBGetter) *DBState {
	return &DBState{
		dbGetter: dbGetter,
	}
}

// Create a new model.
// TODO (stickupkid): Show we wrap the current mongo implementation in a dqlite
// txn. At least we can rollback if mongo fails.
func (s *DBState) Create(ctx context.Context, uuid string) error {
	return s.with(ctx, func(txn *sql.Tx) error {
		stmt := "INSERT INTO model_list (uuid) VALUES (?);"
		result, err := txn.ExecContext(ctx, stmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		if num, err := result.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if num != 1 {
			return errors.Errorf("expected 1 row to be inserted, got %d", num)
		}
		return nil
	})
}

func (s *DBState) with(ctx context.Context, fn func(txn *sql.Tx) error) error {
	db, err := s.dbGetter.ControllerDB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return fn(tx)
	})
}
