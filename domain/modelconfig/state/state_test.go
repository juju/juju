// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	statemodel "github.com/juju/juju/domain/model/state/model"
	"github.com/juju/juju/domain/modelagent"
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
		c.Assert(config, tc.DeepEquals, test.Expected)
	}
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
	c.Assert(len(rval), tc.Equals, 0)
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
		err := st.SetModelConfig(c.Context(), test.Config)
		c.Assert(err, tc.ErrorIsNil)

		config, err := st.ModelConfig(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(config, tc.DeepEquals, test.Config)
	}
}

// TestGetModelAgentVersionAndStreamNotFound is testing that when we ask for the agent
// version and stream of the current model and that data has not been set that a
// [errors.NotFound] error is returned.
func (s *stateSuite) TestGetModelAgentVersionAndStreamNotFound(c *tc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	_, _, err := st.GetModelAgentVersionAndStream(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelAgentVersionAndStream is testing the happy path that when agent
// version and stream is set it is reported back correctly with no errors.
func (s *stateSuite) TestGetModelAgentVersionAndStream(c *tc.C) {
	s.createTestModel(c)

	st := state.NewState(s.TxnRunnerFactory())
	version, stream, err := st.GetModelAgentVersionAndStream(c.Context())
	c.Check(err, tc.ErrorIsNil)
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

	id := coremodel.GenUUID(c)
	cid := uuid.MustNewUUID()
	args := model.ModelDetailArgs{
		UUID:               id,
		AgentStream:        modelagent.AgentStreamReleased,
		AgentVersion:       jujuversion.Current,
		LatestAgentVersion: jujuversion.Current,
		ControllerUUID:     cid,
		Name:               "my-awesome-model",
		Qualifier:          "prod",
		Type:               coremodel.IAAS,
		Cloud:              "aws",
		CloudType:          "ec2",
		CloudRegion:        "myregion",
		CredentialOwner:    coreuser.GenName(c, "myowner"),
		CredentialName:     "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	return id
}
