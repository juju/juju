// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmanager/state"
)

// RegisterModel is responsible for registering the existence of a model in Juju.
// The act of registering a model does not mean the model exists in Juju terms
// for users to deploy applications to, but registers the fact that a model uuid
// exists with a database attached to it. The following errors can occur:
// - errors.NotValid: If the model uuid provided is not valid.
// - modelerrors.AlreadyExists: If the model uuid has already been registered.
func RegisterModel(uuid coremodel.UUID) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		if err := uuid.Validate(); err != nil {
			return fmt.Errorf("model uuid is %w", errors.NotValid)
		}

		return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return state.Create(ctx, uuid, tx)
		})
	}
}
