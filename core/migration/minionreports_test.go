// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
)

type MinionReportsSuite struct {
	coretesting.BaseSuite
}

func TestMinionReportsSuite(t *testing.T) {
	tc.Run(t, new(MinionReportsSuite))
}

func (s *MinionReportsSuite) TestIsZero(c *tc.C) {
	reports := migration.MinionReports{}
	c.Check(reports.IsZero(), tc.IsTrue)
}

func (s *MinionReportsSuite) TestIsZeroIdSet(c *tc.C) {
	reports := migration.MinionReports{
		MigrationId: "foo",
	}
	c.Check(reports.IsZero(), tc.IsFalse)
}

func (s *MinionReportsSuite) TestIsZeroPhaseSet(c *tc.C) {
	reports := migration.MinionReports{
		Phase: migration.QUIESCE,
	}
	c.Check(reports.IsZero(), tc.IsFalse)
}
