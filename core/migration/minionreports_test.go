// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/testing"
)

type MinionReportsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(new(MinionReportsSuite))

func (s *MinionReportsSuite) TestIsZero(c *gc.C) {
	reports := migration.MinionReports{}
	c.Check(reports.IsZero(), jc.IsTrue)
}

func (s *MinionReportsSuite) TestIsZeroIdSet(c *gc.C) {
	reports := migration.MinionReports{
		MigrationId: "foo",
	}
	c.Check(reports.IsZero(), jc.IsFalse)
}

func (s *MinionReportsSuite) TestIsZeroPhaseSet(c *gc.C) {
	reports := migration.MinionReports{
		Phase: migration.QUIESCE,
	}
	c.Check(reports.IsZero(), jc.IsFalse)
}
