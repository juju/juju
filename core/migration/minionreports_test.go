// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
)

type MinionReportsSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(new(MinionReportsSuite))

func (s *MinionReportsSuite) TestIsZero(c *tc.C) {
	reports := migration.MinionReports{}
	c.Check(reports.IsZero(), jc.IsTrue)
}

func (s *MinionReportsSuite) TestIsZeroIdSet(c *tc.C) {
	reports := migration.MinionReports{
		MigrationId: "foo",
	}
	c.Check(reports.IsZero(), jc.IsFalse)
}

func (s *MinionReportsSuite) TestIsZeroPhaseSet(c *tc.C) {
	reports := migration.MinionReports{
		Phase: migration.QUIESCE,
	}
	c.Check(reports.IsZero(), jc.IsFalse)
}
