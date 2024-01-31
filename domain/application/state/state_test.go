// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))
}

func ptr[T any](v T) *T {
	return &v
}

func (s *stateSuite) TestCreateApplicationNoUnits(c *gc.C) {
	err := s.state.UpsertApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var name string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM application").Scan(&name)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "666")
}

func (s *stateSuite) TestCreateApplication(c *gc.C) {
	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.state.UpsertApplication(context.Background(), "666", u)
	c.Assert(err, jc.ErrorIsNil)

	var name string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM application").Scan(&name)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "666")

	var unitID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT unit_id FROM unit").Scan(&unitID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitID, gc.Equals, "foo/666")
}

func (s *stateSuite) TestUpdateApplication(c *gc.C) {
	err := s.state.UpsertApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err = s.state.UpsertApplication(context.Background(), "666", u)
	c.Assert(err, jc.ErrorIsNil)

	var name string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM application").Scan(&name)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "666")

	var unitID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT unit_id FROM unit").Scan(&unitID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitID, gc.Equals, "foo/666")
}

func (s *stateSuite) TestDeleteApplication(c *gc.C) {
	err := s.state.UpsertApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	//s.insertBlockDevice(c, bd, bdUUID, "666")

	err = s.state.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var appCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "666").Scan(&appCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCount, gc.Equals, 0)
}

func (s *stateSuite) TestDeleteApplicationWithUnits(c *gc.C) {
	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.state.UpsertApplication(context.Background(), "666", u)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, applicationerrors.HasUnits)
	c.Assert(err, gc.ErrorMatches, `.*cannot delete application "666" as it still has 1 unit\(s\)`)

	var appCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM application WHERE name=?", "666").Scan(&appCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCount, gc.Equals, 1)
}

func (s *stateSuite) TestAddUnits(c *gc.C) {
	err := s.state.UpsertApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err = s.state.AddUnits(context.Background(), "666", u)
	c.Assert(err, jc.ErrorIsNil)

	var name string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM application").Scan(&name)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "666")

	var unitID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT unit_id FROM unit").Scan(&unitID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitID, gc.Equals, "foo/666")
}

func (s *stateSuite) TestAddUnitsMissingApplication(c *gc.C) {
	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.state.AddUnits(context.Background(), "666", u)
	c.Assert(err, jc.ErrorIs, applicationerrors.NotFound)
}
