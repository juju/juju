// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	databasetesting "github.com/juju/juju/internal/database/testing"
)

type querySuite struct {
	databasetesting.DqliteSuite
}

var _ = tc.Suite(&querySuite{})

func (s *querySuite) TestCreateSchemaTable(c *tc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createSchemaTable(ctx, tx)
	})
	c.Assert(err, tc.ErrorIsNil)

	tableNames := set.NewStrings(readTableNames(c, s.DB())...)
	c.Check(tableNames.Contains("schema"), tc.IsTrue)
}

func (s *querySuite) TestCreateSchemaTableIdempotent(c *tc.C) {
	for i := 0; i < 2; i++ {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			return createSchemaTable(ctx, tx)
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	tableNames := set.NewStrings(readTableNames(c, s.DB())...)
	c.Check(tableNames.Contains("schema"), tc.IsTrue)
}

func (s *querySuite) TestSelectSchemaVersions(c *tc.C) {
	hash0 := computeHash("a")
	hash1 := computeHash("b")
	computed := computeHashes([]Patch{
		{hash: hash0},
		{hash: hash1},
	})

	s.createSchemaTable(c)
	s.expectCurrentVersion(c, 0, []string{})

	s.insertSchemaVersion(c, 1, computed[0])
	s.expectCurrentVersion(c, 1, computed[:1])

	s.insertSchemaVersion(c, 2, computed[1])
	s.expectCurrentVersion(c, 2, computed[:2])
}

func (s *querySuite) TestCheckSchemaVersionsHaveNoHoles(c *tc.C) {
	err := checkSchemaVersionsHaveNoHoles([]versionHash{
		{version: 1, hash: "a"},
		{version: 2, hash: "b"},
	})
	c.Assert(err, tc.ErrorIsNil)

	err = checkSchemaVersionsHaveNoHoles([]versionHash{
		{version: 1, hash: "a"},
		{version: 3, hash: "c"},
	})
	c.Assert(err, tc.ErrorMatches, `missing patches: 1 to 3`)
}

func (s *querySuite) TestCheckSchemaHashesMatch(c *tc.C) {
	err := checkSchemaHashesMatch([]versionHash{
		{version: 1, hash: "a"},
		{version: 2, hash: "b"},
	}, []string{"a", "b"})
	c.Assert(err, tc.ErrorIsNil)

	err = checkSchemaHashesMatch([]versionHash{
		{version: 1, hash: "a"},
		{version: 2, hash: "b"},
	}, []string{"a", "x"})
	c.Assert(err, tc.ErrorMatches, `hash mismatch for version 2`)
}

func (s *querySuite) TestEnsurePatchesAreAppliedWithHigherCurrent(c *tc.C) {
	err := ensurePatchesAreApplied(context.Background(), nil, 10, []Patch{}, nil)
	c.Assert(err, tc.ErrorMatches, `schema version '10' is more recent than expected '0'`)
}

func (s *querySuite) TestEnsurePatchesNoPatches(c *tc.C) {
	err := ensurePatchesAreApplied(context.Background(), nil, 0, []Patch{}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *querySuite) TestEnsurePatches(c *tc.C) {
	s.createSchemaTable(c)

	patches := []Patch{
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
	}

	var called bool
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return ensurePatchesAreApplied(context.Background(), tx, 0, patches, func(i int, statement string) error {
			called = true
			c.Check(i, tc.Equals, 0)
			return nil
		})
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *querySuite) createSchemaTable(c *tc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createSchemaTable(ctx, tx)
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *querySuite) expectCurrentVersion(c *tc.C, expected int, hashes []string) {
	var current int
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		current, err = queryCurrentVersion(ctx, tx, hashes)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(current, tc.Equals, expected)
}

func (s *querySuite) insertSchemaVersion(c *tc.C, version int, hash string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return insertSchemaVersion(ctx, tx, versionHash{
			version: version,
			hash:    hash,
		})
	})
	c.Assert(err, tc.ErrorIsNil)
}

func readTableNames(c *tc.C, db *sql.DB) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	c.Assert(err, tc.ErrorIsNil)

	rows, err := tx.QueryContext(ctx, "SELECT tbl_name FROM sqlite_master")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var table string
		err = rows.Scan(&table)
		c.Assert(err, tc.ErrorIsNil)
		tables = append(tables, table)
	}

	err = tx.Commit()
	c.Assert(err, tc.ErrorIsNil)

	return set.NewStrings(tables...).SortedValues()
}
