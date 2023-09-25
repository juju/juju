// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	databasetesting "github.com/juju/juju/internal/database/testing"
)

type querySuite struct {
	databasetesting.DqliteSuite
}

var _ = gc.Suite(&querySuite{})

func (s *querySuite) TestCreateSchemaTable(c *gc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createSchemaTable(ctx, tx)
	})
	c.Assert(err, jc.ErrorIsNil)

	tableNames := set.NewStrings(readTableNames(c, s.DB())...)
	c.Check(tableNames.Contains("schema"), jc.IsTrue)
}

func (s *querySuite) TestCreateSchemaTableIdempotent(c *gc.C) {
	for i := 0; i < 2; i++ {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			return createSchemaTable(ctx, tx)
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	tableNames := set.NewStrings(readTableNames(c, s.DB())...)
	c.Check(tableNames.Contains("schema"), jc.IsTrue)
}

func (s *querySuite) TestSelectSchemaVersions(c *gc.C) {
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

func (s *querySuite) TestCheckSchemaVersionsHaveNoHoles(c *gc.C) {
	err := checkSchemaVersionsHaveNoHoles([]versionHash{
		{version: 1, hash: "a"},
		{version: 2, hash: "b"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = checkSchemaVersionsHaveNoHoles([]versionHash{
		{version: 1, hash: "a"},
		{version: 3, hash: "c"},
	})
	c.Assert(err, gc.ErrorMatches, `missing patches: 1 to 3`)
}

func (s *querySuite) TestCheckSchemaHashesMatch(c *gc.C) {
	err := checkSchemaHashesMatch([]versionHash{
		{version: 1, hash: "a"},
		{version: 2, hash: "b"},
	}, []string{"a", "b"})
	c.Assert(err, jc.ErrorIsNil)

	err = checkSchemaHashesMatch([]versionHash{
		{version: 1, hash: "a"},
		{version: 2, hash: "b"},
	}, []string{"a", "x"})
	c.Assert(err, gc.ErrorMatches, `hash mismatch for version 2`)
}

func (s *querySuite) TestEnsurePatchesAreAppliedWithHigherCurrent(c *gc.C) {
	err := ensurePatchesAreApplied(context.Background(), nil, 10, []Patch{}, nil)
	c.Assert(err, gc.ErrorMatches, `schema version '10' is more recent than expected '0'`)
}

func (s *querySuite) TestEnsurePatchesNoPatches(c *gc.C) {
	err := ensurePatchesAreApplied(context.Background(), nil, 0, []Patch{}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *querySuite) TestEnsurePatches(c *gc.C) {
	s.createSchemaTable(c)

	patches := []Patch{
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
	}

	var called bool
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return ensurePatchesAreApplied(context.Background(), tx, 0, patches, func(i int) error {
			called = true
			c.Check(i, gc.Equals, 0)
			return nil
		})
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *querySuite) createSchemaTable(c *gc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createSchemaTable(ctx, tx)
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *querySuite) expectCurrentVersion(c *gc.C, expected int, hashes []string) {
	var current int
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		current, err = queryCurrentVersion(ctx, tx, hashes)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(current, gc.Equals, expected)
}

func (s *querySuite) insertSchemaVersion(c *gc.C, version int, hash string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return insertSchemaVersion(ctx, tx, versionHash{
			version: version,
			hash:    hash,
		})
	})
	c.Assert(err, jc.ErrorIsNil)
}

func readTableNames(c *gc.C, db *sql.DB) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	c.Assert(err, jc.ErrorIsNil)

	rows, err := tx.QueryContext(ctx, "SELECT tbl_name FROM sqlite_master")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var table string
		err = rows.Scan(&table)
		c.Assert(err, jc.ErrorIsNil)
		tables = append(tables, table)
	}

	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)

	return set.NewStrings(tables...).SortedValues()
}
