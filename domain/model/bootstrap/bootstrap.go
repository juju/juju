// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/model/state"
)

// CreateModel is responsible for making a new model with all of its associated
// metadata during the bootstrap process.
// If the ModelCreationArgs do not have a credential name set then no cloud
// credential will be associated with the model.
// The following error types can be expected to be returned:
// - modelerrors.AlreadyExists: When the model uuid is already in use or a model
// with the same name and owner already exists.
// - errors.NotFound: When the cloud, cloud region, or credential do not exist.
// - errors.NotValid: When the model uuid is not valid.
func CreateModel(
	uuid model.UUID,
	args model.ModelCreationArgs,
) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		if err := args.Validate(); err != nil {
			return fmt.Errorf("model creation args: %w", err)
		}
		if err := uuid.Validate(); err != nil {
			return fmt.Errorf("invalid model uuid: %w", err)
		}

		return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return state.Create(ctx, uuid, args, tx)
		})
	}
}
