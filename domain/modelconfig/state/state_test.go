// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	statemodel "github.com/juju/juju/domain/model/state/model"
	"github.com/juju/juju/domain/modelconfig/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

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
			c.Context(),
			test.UpdateAttrs,
			test.RemoveAttrs,
		)
		c.Assert(err, tc.ErrorIsNil)

		config, err := st.ModelConfig(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(config, tc.DeepEquals, test.Expected)
	}
}

func (s *stateSuite) TestModelConfigEmpty(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	modelConfig, err := st.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelConfig, tc.DeepEquals, map[string]string{})
}

func (s *stateSuite) TestModelConfigAgentStream(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO agent_version (stream_id, target_version, latest_version) 
                   VALUES (1, ?, ?)`,
			jujuversion.Current.String(), jujuversion.Current.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	modelConfig, err := st.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelConfig, tc.DeepEquals, map[string]string{
		"agent-stream":  "proposed",
		"agent-version": jujuversion.Current.String(),
	})
}

func (s *stateSuite) TestModelConfigAgentStreamWithValueInConfig(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO agent_version (stream_id, target_version, latest_version) 
                   VALUES (1, ?, ?)`,
			jujuversion.Current.String(), jujuversion.Current.String())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO model_config (key, value) 
				   VALUES ('agent-stream', 'devel')`,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	modelConfig, err := st.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelConfig, tc.DeepEquals, map[string]string{
		"agent-stream":  "proposed",
		"agent-version": jujuversion.Current.String(),
	})
}

// TestModelConfigAgentKeysAuthoritative tests that agent_version is the sole
// source of the agent-stream and agent-version keys, whatever competing values
// sort before or after the authoritative ones, and that the view yields
// exactly one row for each key.
func (s *stateSuite) TestModelConfigAgentKeysAuthoritative(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO agent_version (stream_id, target_version, latest_version)
VALUES (1, '4.1.5', '4.1.5')`)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO model_config (key, value) VALUES
	('agent-stream', 'a-before'),
	('agent-version', '0.0.1'),
	('other', 'value')`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	modelConfig, err := st.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelConfig, tc.DeepEquals, map[string]string{
		"agent-stream":  "proposed",
		"agent-version": "4.1.5",
		"other":         "value",
	})

	// Repeat with competing values that sort after the authoritative ones.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model_config SET value = 'z-after' WHERE key = 'agent-stream'`)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
UPDATE model_config SET value = '99.0.0' WHERE key = 'agent-version'`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	modelConfig, err = st.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelConfig, tc.DeepEquals, map[string]string{
		"agent-stream":  "proposed",
		"agent-version": "4.1.5",
		"other":         "value",
	})

	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT count(*) FROM v_model_config WHERE key IN ('agent-stream', 'agent-version')`).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 2)
}

func (s *stateSuite) TestModelConfigHasAttributesNil(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	rval, err := st.ModelConfigHasAttributes(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(rval), tc.Equals, 0)
}

func (s *stateSuite) TestModelConfigHasAttributesEmpty(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	rval, err := st.ModelConfigHasAttributes(c.Context(), []string{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(rval), tc.Equals, 0)
}

func (s *stateSuite) TestModelConfigHasAttributes(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	err := st.UpdateModelConfig(c.Context(), map[string]string{
		"wallyworld": "peachy",
		"foo":        "bar",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	rval, err := st.ModelConfigHasAttributes(c.Context(), []string{"wallyworld", "doesnotexist"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rval, tc.DeepEquals, []string{"wallyworld"})
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
		err := st.SetModelConfig(c.Context(), test.Config)
		c.Assert(err, tc.ErrorIsNil)

		config, err := st.ModelConfig(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(config, tc.DeepEquals, test.Config)
	}
}

// TestGetModelAgentVersionAndStreamNotFound is testing that when we ask for the agent
// version and stream of the current model and that data has not been set that a
// [errors.NotFound] error is returned.
func (s *stateSuite) TestGetModelAgentVersionAndStreamNotFound(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	_, _, err := st.GetModelAgentVersionAndStream(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelAgentVersionAndStream is testing the happy path that when agent
// version and stream is set it is reported back correctly with no errors.
func (s *stateSuite) TestGetModelAgentVersionAndStream(c *tc.C) {
	s.createTestModel(c)

	st := state.NewState(s.TxnRunnerFactory())
	version, stream, err := st.GetModelAgentVersionAndStream(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version, tc.Equals, jujuversion.Current.String())
	c.Check(stream, tc.Equals, "released")
}

func (s *stateSuite) TestCheckSpace(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	db := s.DB()

	_, err := db.Exec("INSERT INTO space (uuid, name) VALUES ('1', 'foo')")
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.SpaceExists(c.Context(), "bar")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)

	exists, err = st.SpaceExists(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}

func (s *stateSuite) createTestModel(c *tc.C) coremodel.UUID {
	runner := s.TxnRunnerFactory()
	state := statemodel.NewState(runner, loggertesting.WrapCheckLog(c))

	id := tc.Must0(c, coremodel.NewUUID)
	cid := uuid.MustNewUUID()
	args := model.ModelDetailArgs{
		UUID:               id,
		AgentStream:        domainagentbinary.AgentStreamReleased,
		AgentVersion:       jujuversion.Current,
		LatestAgentVersion: jujuversion.Current,
		ControllerUUID:     cid,
		Name:               "my-awesome-model",
		Qualifier:          "prod",
		Type:               coremodel.IAAS,
		Cloud:              "aws",
		CloudType:          "ec2",
		CloudRegion:        "myregion",
		CredentialOwner:    usertesting.GenNewName(c, "myowner"),
		CredentialName:     "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	return id
}
