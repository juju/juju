// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/modelconfig/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) TestModelConfigUpdate(c *tc.C) {
	// tests are purposefully additive in this approach.
	tests := []struct {
		UpdateAttrs map[string]string
		RemoveAttrs []string
		Expected    map[string]string
	}{
		{
			UpdateAttrs: map[string]string{
				"wallyworld": "peachy",
			},
			Expected: map[string]string{
				"wallyworld": "peachy",
			},
		},
		{
			RemoveAttrs: []string{"wallyworld"},
			Expected:    map[string]string{},
		},
		{
			UpdateAttrs: map[string]string{
				"wallyworld": "peachy",
				"foo":        "bar",
			},
			Expected: map[string]string{
				"wallyworld": "peachy",
				"foo":        "bar",
			},
		},
		{
			Expected: map[string]string{
				"wallyworld": "peachy",
				"foo":        "bar",
			},
		},
		{
			UpdateAttrs: map[string]string{
				"wallyworld": "peachy1",
			},
			RemoveAttrs: []string{"foo"},
			Expected: map[string]string{
				"wallyworld": "peachy1",
			},
		},
	}

	st := state.NewState(s.TxnRunnerFactory())

	for _, test := range tests {
		err := st.UpdateModelConfig(
			context.Background(),
			test.UpdateAttrs,
			test.RemoveAttrs,
		)
		c.Assert(err, tc.ErrorIsNil)

		config, err := st.ModelConfig(context.Background())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(config, tc.DeepEquals, test.Expected)
	}
}

func (s *stateSuite) TestModelConfigHasAttributesNil(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	rval, err := st.ModelConfigHasAttributes(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(rval), tc.Equals, 0)
}

func (s *stateSuite) TestModelConfigHasAttributesEmpty(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	rval, err := st.ModelConfigHasAttributes(context.Background(), []string{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(rval), tc.Equals, 0)
}

func (s *stateSuite) TestModelConfigHasAttributes(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	err := st.UpdateModelConfig(context.Background(), map[string]string{
		"wallyworld": "peachy",
		"foo":        "bar",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	rval, err := st.ModelConfigHasAttributes(context.Background(), []string{"wallyworld", "doesnotexist"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rval, tc.DeepEquals, []string{"wallyworld"})
}

func (s *stateSuite) TestSetModelConfig(c *tc.C) {
	tests := []struct {
		Config map[string]string
	}{
		{
			Config: map[string]string{
				"foo": "bar",
			},
		},
		{
			Config: map[string]string{
				"status": "healthy",
				"one":    "two",
			},
		},
	}

	// We don't want to make new state for each test as set explicitly overrides
	// so we want to test that this is happening between tests.
	st := state.NewState(s.TxnRunnerFactory())

	for _, test := range tests {
		err := st.SetModelConfig(context.Background(), test.Config)
		c.Assert(err, tc.ErrorIsNil)

		config, err := st.ModelConfig(context.Background())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(config, tc.DeepEquals, test.Config)
	}
}

// TestGetModelAgentVersionAndStreamNotFound is testing that when we ask for the agent
// version and stream of the current model and that data has not been set that a
// [errors.NotFound] error is returned.
func (s *stateSuite) TestGetModelAgentVersionAndStreamNotFound(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	version, stream, err := st.GetModelAgentVersionAndStream(context.Background())
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(version, tc.Equals, "")
	c.Check(stream, tc.Equals, "")
}

// TestGetModelAgentVersionAndStream is testing the happy path that when agent
// version and stream is set it is reported back correctly with no errors.
func (s *stateSuite) TestGetModelAgentVersionAndStream(c *tc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO agent_version (stream_id, target_version) VALUES (0, '1.2.3')")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	version, stream, err := st.GetModelAgentVersionAndStream(context.Background())
	c.Check(err, tc.ErrorIsNil)
	c.Check(version, tc.Equals, "1.2.3")
	c.Check(stream, tc.Equals, "released")
}

func (s *stateSuite) TestCheckSpace(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	db := s.DB()

	_, err := db.Exec("INSERT INTO space (uuid, name) VALUES ('1', 'foo')")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.SpaceExists(context.Background(), "bar")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)

	exists, err = st.SpaceExists(context.Background(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}
