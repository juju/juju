// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

// Set the agent version for the given model in the DB.
func (s *stateSuite) setupModel(c *gc.C, vers string) {
	modelUUID := modeltesting.GenModelUUID(c).String()
	controllerUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO agent_version (target_version) values (?)", vers)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type, is_controller_model) values (?, ?, ?, ?, ?, ?, ?)",
			modelUUID, controllerUUID, "fred", "iaas", "aws", "ec2", true)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO model_config (key, value) values (?, ?)", "agent-stream", "released")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetModelVersionInfo(c *gc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	s.setupModel(c, expectedVersion.String())

	obtainedVersion, isController, err := st.GetModelVersionInfo(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedVersion, jc.DeepEquals, expectedVersion)
	c.Check(isController, jc.IsTrue)
}

func (s *stateSuite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, _, err := st.GetModelVersionInfo(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.AgentVersionNotFound)
}

func (s *stateSuite) TestGetModelAgentVersionCantParseVersion(c *gc.C) {
	s.setupModel(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, _, err := st.GetModelVersionInfo(context.Background())
	c.Check(err, gc.ErrorMatches, `parsing agent version: invalid version "invalid-version".*`)
}

func (s *stateSuite) TestSetTargetAgentVersion(c *gc.C) {
	s.setupModel(c, "1.1.1")

	st := NewState(s.TxnRunnerFactory())
	err := st.SetTargetAgentVersion(context.Background(), semversion.MustParse("6.6.6"), nil)
	c.Assert(err, jc.ErrorIsNil)

	var vers, stream string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT target_version FROM agent_version").Scan(&vers)
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT value FROM model_config WHERE key='agent-stream'").Scan(&stream)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, gc.Equals, "6.6.6")
	c.Assert(stream, gc.Equals, "released")
}

func ptr[T any](v T) *T {
	return &v
}

func (s *stateSuite) TestSetTargetAgentVersionWithAgentStream(c *gc.C) {
	s.setupModel(c, "1.1.1")

	st := NewState(s.TxnRunnerFactory())
	err := st.SetTargetAgentVersion(context.Background(), semversion.MustParse("6.6.6"), ptr("proposed"))
	c.Assert(err, jc.ErrorIsNil)

	var vers, stream string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT target_version FROM agent_version").Scan(&vers)
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT value FROM model_config WHERE key='agent-stream'").Scan(&stream)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, gc.Equals, "6.6.6")
	c.Assert(stream, gc.Equals, "proposed")
}
