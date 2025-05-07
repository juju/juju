// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/changestream"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type mapperSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&mapperSuite{})

func (s *mapperSuite) TestUuidToNameMapper(c *tc.C) {
	uuid0 := uuid.MustNewUUID().String()
	uuid1 := uuid.MustNewUUID().String()
	uuid2 := uuid.MustNewUUID().String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO net_node (uuid) VALUES (?)"
		if _, err := tx.ExecContext(ctx, stmt, uuid0); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, stmt, uuid1); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, stmt, uuid2); err != nil {
			return err
		}

		stmt = "INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, ?)"
		if _, err := tx.ExecContext(ctx, stmt, uuid0, "0", uuid0, 0); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, stmt, uuid1, "1", uuid1, 0); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, stmt, uuid2, "0/lxd/0", uuid2, 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	changesIn := []changestream.ChangeEvent{
		changeEventShim{
			changeType: 1,
			namespace:  "machine",
			changed:    uuid0,
		},
		changeEventShim{
			changeType: 2,
			namespace:  "machine",
			changed:    uuid1,
		},
	}

	changesOut, err := uuidToNameMapper(noContainersFilter)(context.Background(), s.TxnRunner(), changesIn)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(changesOut, jc.SameContents, []changestream.ChangeEvent{
		changeEventShim{
			changeType: 1,
			namespace:  "machine",
			changed:    "0",
		},
		changeEventShim{
			changeType: 2,
			namespace:  "machine",
			changed:    "1",
		},
	})
}
