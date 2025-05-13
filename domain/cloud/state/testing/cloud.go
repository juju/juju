// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/uuid"
)

// CreateTestCloud is responsible for establishing a test cloud within the
// DQlite database.
func CreateTestCloud(
	c *tc.C,
	txnRunner database.TxnRunnerFactory,
	cloud cloud.Cloud,
) uuid.UUID {
	runner, err := txnRunner()
	c.Assert(err, tc.ErrorIsNil)

	cloudUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {

		_, err = tx.ExecContext(ctx, `
			INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify)
			SELECT ?, ?, cloud_type.id, "", true
			FROM cloud_type
			WHERE type = ?
		`, cloudUUID.String(), cloud.Name, cloud.Type)
		if err != nil {
			fmt.Println("this error")
			return err
		}

		for _, authType := range cloud.AuthTypes {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO cloud_auth_type (cloud_uuid, auth_type_id)
				SELECT ?, auth_type.id
				FROM auth_type
				WHERE type = ?
			`, cloudUUID.String(), authType)
			if err != nil {
				fmt.Println("this error2")
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return cloudUUID
}
