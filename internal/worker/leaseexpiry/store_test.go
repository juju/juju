// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"
	_ "github.com/mattn/go-sqlite3"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/leaseexpiry"
)

type storeSuite struct {
	testhelpers.IsolationSuite
}

func TestStoreSuite(t *testing.T) {
	tc.Run(t, &storeSuite{})
}

func (s *storeSuite) TestExpireLeasesUsesStdTxnNoRetry(c *tc.C) {
	db, err := sql.Open("sqlite3", ":memory:")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Check(db.Close(), tc.ErrorIsNil) }()

	_, err = db.ExecContext(c.Context(), `
CREATE TABLE lease (
    uuid TEXT PRIMARY KEY,
    expiry DATETIME NOT NULL
);
CREATE TABLE lease_pin (
    uuid TEXT PRIMARY KEY,
    lease_uuid TEXT NOT NULL
);`)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(c.Context(), `
INSERT INTO lease (uuid, expiry) VALUES
    ('future', datetime('now', '+2 minutes')),
    ('expired', datetime('now', '-2 minutes')),
    ('pinned', datetime('now', '-2 minutes'));
INSERT INTO lease_pin (uuid, lease_uuid) VALUES ('pin', 'pinned');`)
	c.Assert(err, tc.ErrorIsNil)

	runner := noRetryTxnRunner{
		TxnRunner: noopTxnRunner{},
		db:        db,
	}
	store, err := leaseexpiry.NewStore(
		c.Context(),
		stubDBGetter{runner: runner},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)

	err = store.ExpireLeases(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	rows, err := db.QueryContext(c.Context(), "SELECT uuid FROM lease ORDER BY uuid")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Check(rows.Close(), tc.ErrorIsNil) }()

	var got []string
	for rows.Next() {
		var leaseUUID string
		c.Assert(rows.Scan(&leaseUUID), tc.ErrorIsNil)
		got = append(got, leaseUUID)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, []string{"future", "pinned"})
}

type noRetryTxnRunner struct {
	coredatabase.TxnRunner
	db *sql.DB
}

func (r noRetryTxnRunner) StdTxnNoRetry(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return database.StdTxn(ctx, r.db, fn)
}
