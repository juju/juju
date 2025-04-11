// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
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

func (s *migrationStateSuite) TestGetApplicationsForExport(c *gc.C) {
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
			Subordinate: false,
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationsForExportMany(c *gc.C) {
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
			Subordinate: false,
		})
	}

	apps, err := st.GetApplicationsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.DeepEquals, want)
}

func (s *migrationStateSuite) TestGetApplicationsForExportDeadOrDying(c *gc.C) {
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
			Subordinate: false,
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
			Subordinate: false,
		},
	}

	apps, err := st.GetApplicationsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.DeepEquals, want)
}

func (s *migrationStateSuite) TestGetApplicationsForExportWithNoApplications(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	apps, err := st.GetApplicationsForExport(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.HasLen, 0)
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExport(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
		Password: &application.PasswordInfo{
			PasswordHash:  "password",
			HashAlgorithm: 0,
		},
	})

	unitUUID, err := st.GetUnitUUIDByName(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	units, err := st.GetApplicationUnitsForExport(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(units, jc.DeepEquals, []application.ExportUnit{
		{
			UUID:    unitUUID,
			Name:    "foo/0",
			Machine: machine.Name("0"),
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportNoUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createApplication(c, "foo", life.Alive)

	units, err := st.GetApplicationUnitsForExport(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(units, jc.DeepEquals, []application.ExportUnit{})
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportDying(c *gc.C) {
	// We shouldn't export units that are in the dying state, but the old code
	// doesn't prohibit this.

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
		Password: &application.PasswordInfo{
			PasswordHash:  "password",
			HashAlgorithm: 0,
		},
	})

	unitUUID, err := st.GetUnitUUIDByName(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE uuid = ?", life.Dying, unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	units, err := st.GetApplicationUnitsForExport(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(units, jc.DeepEquals, []application.ExportUnit{
		{
			UUID:    unitUUID,
			Name:    "foo/0",
			Machine: machine.Name("0"),
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportDead(c *gc.C) {
	// We shouldn't export units that are in the dead state, but the old code
	// doesn't prohibit this.

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
		Password: &application.PasswordInfo{
			PasswordHash:  "password",
			HashAlgorithm: 0,
		},
	})

	unitUUID, err := st.GetUnitUUIDByName(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE uuid = ?", life.Dead, unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	units, err := st.GetApplicationUnitsForExport(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(units, jc.DeepEquals, []application.ExportUnit{
		{
			UUID:    unitUUID,
			Name:    "foo/0",
			Machine: machine.Name("0"),
		},
	})
}

func (s *unitStateSuite) TestInsertMigratingIAASUnits(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, _, err := s.state.insertMachineAndNetNode(context.Background(), tx, machine.Name("0"))
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.InsertMigratingIAASUnits(context.Background(), appID, application.ImportUnitArg{
		UnitName: "foo/666",
		Machine:  "0",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, appID)
}

func (s *unitStateSuite) TestInsertMigratingCAASUnits(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingCAASUnits(context.Background(), appID, application.ImportUnitArg{
		UnitName: "foo/666",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, appID)
}

func (s *unitStateSuite) assertInsertMigratingUnits(c *gc.C, appID coreapplication.ID) {
	var unitName string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM unit WHERE application_uuid=?", appID).Scan(&unitName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitName, gc.Equals, "foo/666")
}
