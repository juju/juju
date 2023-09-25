// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/testing"
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

// ApplyDDLForRunner is responsible for applying the controller schema to the
// given database.
func (s *ControllerSuite) ApplyDDLForRunner(c *gc.C, runner coredatabase.TxnRunner) {
	s.DqliteSuite.ApplyDDLForRunner(c, &SchemaApplier{
		schema: schema.ControllerDDL(0x2dc171858c3155be),
	}, runner)
}
