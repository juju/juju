// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/model"
	modelmanagerstate "github.com/juju/juju/domain/modelmanager/state"
)

// InsertModel is a bootstrap convenience function for inserting a new model into
// the controllers model list.
func InsertModel(uuid model.UUID) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return modelmanagerstate.Create(ctx, uuid, tx)
		})
	}
}
