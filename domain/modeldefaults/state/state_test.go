// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/config"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

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
		config.TypeKey: "iaas",
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
