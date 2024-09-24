// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/config"
)

type stateSuite struct {
	schematesting.ControllerSuite

	modelUUID model.UUID
}

var _ = gc.Suite(&stateSuite{})

func (m *stateSuite) SetUpTest(c *gc.C) {
	m.ControllerSuite.SetUpTest(c)
	m.modelUUID = modelstatetesting.CreateTestModel(c, m.TxnRunnerFactory(), "model-defaults")
}

// TestModelMetadataDefaults is asserting the happy path of model metadata
// defaults.
func (m *stateSuite) TestModelMetadataDefaults(c *gc.C) {
	uuid := modelstatetesting.CreateTestModel(c, m.TxnRunnerFactory(), "test")
	st := NewState(m.TxnRunnerFactory())
	defaults, err := st.ModelMetadataDefaults(context.Background(), uuid)
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		config.NameKey: "test",
		config.UUIDKey: uuid.String(),
		config.TypeKey: "ec2",
	})
}

// TestModelMetadataDefaultsNoModel is asserting that if we ask for the model
// metadata defaults for a model that doesn't exist we get back a
// [modelerrors.NotFound] error.
func (m *stateSuite) TestModelMetadataDefaultsNoModel(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	st := NewState(m.TxnRunnerFactory())
	defaults, err := st.ModelMetadataDefaults(context.Background(), uuid)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
	c.Check(len(defaults), gc.Equals, 0)
}

// TestGetModelCloudType asserts that the cloud type for a created model is
// correct.
func (m *stateSuite) TestGetModelCloudType(c *gc.C) {
	ct, err := NewState(m.TxnRunnerFactory()).ModelCloudType(
		context.Background(), m.modelUUID,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ct, gc.Equals, "ec2")
}

// TestGetModelCloudTypModelNotFound is asserting that when no model exists we
// get back a [modelerrors.NotFound] error when querying for a model's cloud
// type.
func (m *stateSuite) TestGetModelCloudTypeModelNotFound(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	_, err := NewState(m.TxnRunnerFactory()).ModelCloudType(
		context.Background(), modelUUID,
	)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}
