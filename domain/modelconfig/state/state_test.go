// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/modelconfig/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestModelConfigUpdate(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)

		config, err := st.ModelConfig(context.Background())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(config, jc.DeepEquals, test.Expected)
	}
}

func (s *stateSuite) TestModelConfigHasAttributesNil(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	rval, err := st.ModelConfigHasAttributes(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(rval), gc.Equals, 0)
}

func (s *stateSuite) TestModelConfigHasAttributesEmpty(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	rval, err := st.ModelConfigHasAttributes(context.Background(), []string{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(rval), gc.Equals, 0)
}

func (s *stateSuite) TestModelConfigHasAttributes(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	err := st.UpdateModelConfig(context.Background(), map[string]string{
		"wallyworld": "peachy",
		"foo":        "bar",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	rval, err := st.ModelConfigHasAttributes(context.Background(), []string{"wallyworld", "doesnotexist"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rval, gc.DeepEquals, []string{"wallyworld"})
}

func (s *stateSuite) TestSetModelConfig(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)

		config, err := st.ModelConfig(context.Background())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(config, jc.DeepEquals, test.Config)
	}
}
