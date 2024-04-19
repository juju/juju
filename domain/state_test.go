// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	gc "gopkg.in/check.v1"

	. "github.com/juju/juju/domain/query"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type TestType struct {
	Name string `db:"name"`
}

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestStateBaseGetDB(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)
}

func (s *stateSuite) TestStateBaseGetDBNilFactory(c *gc.C) {
	base := NewStateBase(nil)
	_, err := base.DB()
	c.Assert(err, gc.ErrorMatches, `nil getDB`)
}

func (s *stateSuite) TestStateBasePrepare(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	// Prepare new query.
	stmt1, err := base.Prepare("SELECT name AS &M.* FROM sqlite_schema", sqlair.M{})
	c.Assert(err, gc.IsNil)
	// Validate prepared statement works as expected.
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		results := sqlair.M{}
		err := tx.Query(ctx, stmt1).Get(results)
		if err != nil {
			return err
		}
		c.Assert(results["name"], gc.Equals, "schema")
		return nil
	})
	c.Assert(err, gc.IsNil)

	// Retrieve previous statement.
	stmt2, err := base.Prepare("SELECT name AS &M.* FROM sqlite_schema", sqlair.M{})
	c.Assert(err, gc.IsNil)
	c.Assert(stmt1, gc.Equals, stmt2)
}

func (s *stateSuite) TestStateBasePrepareKeyClash(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	// Prepare statement with TestType.
	{
		type TestType struct {
			WrongName string `db:"type"`
		}
		_, err := base.Prepare("SELECT &TestType.* FROM sqlite_schema", TestType{})
		c.Assert(err, gc.IsNil)
	}

	// Prepare statement with a different type of the same name, this will
	// retrieve the previously prepared statement which used the shadowed type.
	stmt, err := base.Prepare("SELECT &TestType.* FROM sqlite_schema", TestType{})

	// Try and run a query.
	c.Assert(err, gc.IsNil)
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		results := TestType{}
		err := tx.Query(ctx, stmt).Get(&results)
		if err != nil {
			return err
		}
		c.Assert(results.Name, gc.Equals, "schema")
		return nil
	})
	c.Assert(err, gc.ErrorMatches, `cannot get result: parameter with type "domain.TestType" missing, have type with same name: "domain.TestType"`)
}

func (s *stateSuite) TestStateQueryRowSuccess(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	var result TestType
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := base.QueryRow(ctx, tx, "SELECT &TestType.* FROM sqlite_schema", Out(&result))
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, gc.IsNil)
	c.Check(result.Name, gc.Equals, "schema")
}

func (s *stateSuite) TestStateQueryRowWrongTypeError(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	var result []TestType
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := base.QueryRow(ctx, tx, "SELECT &TestType.* FROM sqlite_schema", Out(&result))
		if err != nil {
			return err
		}
		return nil
	})
	c.Check(err, gc.ErrorMatches, ".*cannot use anonymous slice")
	c.Check(result, gc.IsNil)
}

func (s *stateSuite) TestStateQuerySuccess(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	var results []TestType
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := base.Query(ctx, tx, "SELECT &TestType.* FROM sqlite_schema WHERE name in ('schema')", OutM(&results))
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Name, gc.Equals, "schema")
}

func (s *stateSuite) TestStateQueryWrongTypeError(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	var result TestType
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := base.Query(ctx, tx, "SELECT &TestType.* FROM sqlite_schema WHERE name in ('schema')", Out(&result))
		if err != nil {
			return err
		}
		return nil
	})
	c.Check(err, gc.ErrorMatches, ".*need pointer to slice, got pointer to struct")
	c.Check(result.Name, gc.Equals, "")
}

func (s *stateSuite) TestStateExecSuccess(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	arg := TestType{Name: "name"}
	var res sql.Result
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := base.Exec(ctx, tx, "CREATE TABLE dummy (name string)")
		if err != nil {
			return err
		}

		res, err = base.Exec(ctx, tx, "INSERT INTO dummy VALUES ($TestType.name)", In(arg))
		return err
	})
	c.Assert(err, gc.IsNil)

	count, err := res.RowsAffected()
	c.Assert(err, gc.IsNil)
	c.Check(count, gc.Equals, int64(1))
}

func (s *stateSuite) TestStateExecWrongProcessorError(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	arg := TestType{Name: "name"}
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := base.Exec(ctx, tx, "CREATE TABLE dummy (name string)")
		if err != nil {
			return err
		}

		_, err = base.Exec(ctx, tx, "INSERT INTO dummy VALUES ($TestType.name)", Out(&arg))
		return err
	})
	c.Assert(err, gc.ErrorMatches, "invalid input parameter.*")
}
