// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
)

// DummyCloudOpt is a db bootstrap option which inserts the dummy cloud type.
var DummyCloudOpt = func(ctx context.Context, db database.TxnRunner) error {
	return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO cloud_type VALUES (666, "dummy")`)
		return err
	}))
}
