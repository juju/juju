// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
	"github.com/juju/juju/domain/schema"
)

// ControllerSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.DqliteSuite
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &SchemaApplier{
		schema: schema.ControllerDDL(0x2dc171858c3155be),
	})
}
