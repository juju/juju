// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"
	"go.uber.org/goleak"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestStateBaseGetDB(c *tc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(db, tc.NotNil)
}

func (s *stateSuite) TestStateBaseGetDBNilFactory(c *tc.C) {
	base := NewStateBase(nil)
	_, err := base.DB(c.Context())
	c.Assert(err, tc.ErrorMatches, `nil getDB`)
}

func (s *stateSuite) TestStateBasePrepare(c *tc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(db, tc.NotNil)

	// Prepare new query.
	stmt1, err := base.Prepare("SELECT name AS &M.* FROM sqlite_schema", sqlair.M{})
	c.Assert(err, tc.ErrorIsNil)
	// Validate prepared statement works as expected.
	var name any
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		results := sqlair.M{}
		err := tx.Query(ctx, stmt1).Get(results)
		if err != nil {
			return err
		}
		name = results["name"]
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "schema")

	// Retrieve previous statement.
	stmt2, err := base.Prepare("SELECT name AS &M.* FROM sqlite_schema", sqlair.M{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stmt1, tc.Equals, stmt2)
}

func (s *stateSuite) TestStateBasePrepareKeyClash(c *tc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(db, tc.NotNil)

	// Prepare statement with TestType.
	{
		type TestType struct {
			WrongName string `db:"type"`
		}
		_, err := base.Prepare("SELECT &TestType.* FROM sqlite_schema", TestType{})
		c.Assert(err, tc.ErrorIsNil)
	}

	// Prepare statement with a different type of the same name, this will
	// retrieve the previously prepared statement which used the shadowed type.
	type TestType struct {
		Name string `db:"name"`
	}
	stmt, err := base.Prepare("SELECT &TestType.* FROM sqlite_schema", TestType{})
	c.Assert(err, tc.ErrorIsNil)

	// Try and run a query.
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var results TestType
		return tx.Query(ctx, stmt).Get(&results)
	})
	c.Assert(err, tc.ErrorMatches, `cannot get result: parameter with type "domain.TestType" missing, have type with same name: "domain.TestType"`)
}
