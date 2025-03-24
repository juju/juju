// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationStateSuite struct {
	baseSuite
}

var _ = gc.Suite(&migrationStateSuite{})

func (s *migrationStateSuite) TestExportApplications(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createApplication(c, "foo", life.Alive)
	charmID, err := st.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	apps, err := st.GetApplicationsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.DeepEquals, []application.ExportApplication{
		{
			UUID:      id,
			CharmUUID: charmID,
			ModelType: model.IAAS,
			Name:      "foo",
			Life:      life.Alive,
			CharmLocator: charm.CharmLocator{
				Name:     "foo",
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			PasswordHash: "password",
			Placement:    "placement",
			Subordinate:  false,
			Exposed:      false,
		},
	})
}

func (s *migrationStateSuite) TestExportApplicationsMany(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	var want []application.ExportApplication

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("foo%d", i)
		id := s.createApplication(c, name, life.Alive)
		charmID, err := st.GetCharmIDByApplicationName(context.Background(), name)
		c.Assert(err, jc.ErrorIsNil)

		want = append(want, application.ExportApplication{
			UUID:      id,
			CharmUUID: charmID,
			ModelType: model.IAAS,
			Name:      name,
			Life:      life.Alive,
			CharmLocator: charm.CharmLocator{
				Name:     name,
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			PasswordHash: "password",
			Placement:    "placement",
			Subordinate:  false,
			Exposed:      false,
		})
	}

	apps, err := st.GetApplicationsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.DeepEquals, want)
}

func (s *migrationStateSuite) TestExportApplicationsDeadOrDying(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	// The prior state implementation allows for applications to be in the
	// Dying or Dead state. This test ensures that these states are exported
	// correctly.
	// TODO (stickupkid): We should just skip these applications in the export.

	id0 := s.createApplication(c, "foo0", life.Dying)
	charmID0, err := st.GetCharmIDByApplicationName(context.Background(), "foo0")
	c.Assert(err, jc.ErrorIsNil)

	id1 := s.createApplication(c, "foo1", life.Dead)
	charmID1, err := st.GetCharmIDByApplicationName(context.Background(), "foo1")
	c.Assert(err, jc.ErrorIsNil)

	want := []application.ExportApplication{
		{
			UUID:      id0,
			CharmUUID: charmID0,
			ModelType: model.IAAS,
			Name:      "foo0",
			Life:      life.Dying,
			CharmLocator: charm.CharmLocator{
				Name:     "foo0",
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			PasswordHash: "password",
			Placement:    "placement",
			Subordinate:  false,
			Exposed:      false,
		},
		{
			UUID:      id1,
			CharmUUID: charmID1,
			ModelType: model.IAAS,
			Name:      "foo1",
			Life:      life.Dead,
			CharmLocator: charm.CharmLocator{
				Name:     "foo1",
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			PasswordHash: "password",
			Placement:    "placement",
			Subordinate:  false,
			Exposed:      false,
		},
	}

	apps, err := st.GetApplicationsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.DeepEquals, want)
}

func (s *migrationStateSuite) TestExportApplicationsWithNoApplications(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	apps, err := st.GetApplicationsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.HasLen, 0)
}
