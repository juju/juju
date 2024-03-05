// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	"github.com/canonical/sqlair"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

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
	type TestType struct {
		Name string `db:"name"`
	}
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
