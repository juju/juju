// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestSetTracingConfig(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	tracingConfig := map[string]string{
		"one-key": "one-value",
	}
	err := state.SetTracingConfig(c.Context(), tracingConfig, nil)
	c.Assert(err, tc.ErrorIsNil)

	expectedTracingConfig := map[string]string{
		"one-key": "one-value",
	}

	tracingConfigFromDB, err := state.GetTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tracingConfigFromDB, tc.DeepEquals, expectedTracingConfig)
}

func (s *stateSuite) TestSetTracingConfigWithDeletion(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	tracingConfig := map[string]string{
		"one-key":   "one-value",
		"two-key":   "two-value",
		"three-key": "three-value",
	}
	err := state.SetTracingConfig(c.Context(), tracingConfig, nil)
	c.Assert(err, tc.ErrorIsNil)

	tracingConfig = map[string]string{
		"four-key": "four-value",
	}
	deletionKeys := []string{"two-key", "three-key"}

	err = state.SetTracingConfig(c.Context(), tracingConfig, deletionKeys)
	c.Assert(err, tc.ErrorIsNil)

	expectedTracingConfig := map[string]string{
		"one-key":  "one-value",
		"four-key": "four-value",
	}

	tracingConfigFromDB, err := state.GetTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tracingConfigFromDB, tc.DeepEquals, expectedTracingConfig)
}
