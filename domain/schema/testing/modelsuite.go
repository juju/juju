// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/testing"
)

// ModelSuite is used to provide an in-memory sql.DB reference to tests.
// It is pre-populated with the model schema.
type ModelSuite struct {
	testing.DqliteSuite

	modelUUID string
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the model schema.
func (s *ModelSuite) SetUpTest(c *gc.C) {
	s.modelUUID = utils.MustNewUUID().String()

	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &SchemaApplier{
		Schema: schema.ModelDDL(),
	})
}

func (s *ModelSuite) ModelUUID() string {
	return s.modelUUID
}
