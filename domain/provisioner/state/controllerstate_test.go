// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite

	state *ControllerState
}

func TestControllerStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

func (s *controllerStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewControllerState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

// runQuery executes an SQL statement for test setup.
func (s *controllerStateSuite) runQuery(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %v)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// TestGetControllerConfigEmpty verifies that an empty controller_config
// table returns an empty map.
func (s *controllerStateSuite) TestGetControllerConfigEmpty(c *tc.C) {
	result, err := s.state.GetControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetControllerConfigPopulated verifies that controller_config rows
// are returned as a key-value map.
func (s *controllerStateSuite) TestGetControllerConfigPopulated(c *tc.C) {
	s.runQuery(c, `INSERT INTO controller_config ("key", value) VALUES (?,?)`,
		"controller-uuid", "ctrl-uuid-abc")
	s.runQuery(c, `INSERT INTO controller_config ("key", value) VALUES (?,?)`,
		"api-port", "17070")

	result, err := s.state.GetControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 2)
	c.Check(result["controller-uuid"], tc.Equals, "ctrl-uuid-abc")
	c.Check(result["api-port"], tc.Equals, "17070")
}

// TestGetControllerConfigSingleEntry verifies a single config entry.
func (s *controllerStateSuite) TestGetControllerConfigSingleEntry(c *tc.C) {
	s.runQuery(c, `INSERT INTO controller_config ("key", value) VALUES (?,?)`,
		"model-logfile-max-size", "10M")

	result, err := s.state.GetControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 1)
	c.Check(result["model-logfile-max-size"], tc.Equals, "10M")
}
