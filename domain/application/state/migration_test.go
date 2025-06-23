// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationStateSuite struct {
	baseSuite
}

func TestMigrationStateSuite(t *testing.T) {
	tc.Run(t, &migrationStateSuite{})
}

func (s *migrationStateSuite) TestGetApplicationsForExport(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createIAASApplication(c, "foo", life.Alive)
	charmID, err := st.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	apps, err := st.GetApplicationsForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apps, tc.DeepEquals, []application.ExportApplication{
		{
			UUID:      id,
			CharmUUID: charmID,
			Name:      "foo",
			Life:      life.Alive,
			CharmLocator: charm.CharmLocator{
				Name:     "foo",
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			Subordinate: false,
			EndpointBindings: map[string]network.SpaceUUID{
				"":          network.AlphaSpaceId,
				"endpoint":  network.AlphaSpaceId,
				"extra":     network.AlphaSpaceId,
				"juju-info": network.AlphaSpaceId,
				"misc":      network.AlphaSpaceId,
			},
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationsForExportMany(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	var want []application.ExportApplication

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("foo%d", i)
		id := s.createIAASApplication(c, name, life.Alive)
		charmID, err := st.GetCharmIDByApplicationName(c.Context(), name)
		c.Assert(err, tc.ErrorIsNil)

		want = append(want, application.ExportApplication{
			UUID:      id,
			CharmUUID: charmID,
			Name:      name,
			Life:      life.Alive,
			CharmLocator: charm.CharmLocator{
				Name:     name,
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			Subordinate: false,
			EndpointBindings: map[string]network.SpaceUUID{
				"":          network.AlphaSpaceId,
				"endpoint":  network.AlphaSpaceId,
				"extra":     network.AlphaSpaceId,
				"juju-info": network.AlphaSpaceId,
				"misc":      network.AlphaSpaceId,
			},
		})
	}

	apps, err := st.GetApplicationsForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apps, tc.DeepEquals, want)
}

func (s *migrationStateSuite) TestGetApplicationsForExportDeadOrDying(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	// The prior state implementation allows for applications to be in the
	// Dying or Dead state. This test ensures that these states are exported
	// correctly.
	// TODO (stickupkid): We should just skip these applications in the export.

	id0 := s.createIAASApplication(c, "foo0", life.Dying)
	charmID0, err := st.GetCharmIDByApplicationName(c.Context(), "foo0")
	c.Assert(err, tc.ErrorIsNil)

	id1 := s.createIAASApplication(c, "foo1", life.Dead)
	charmID1, err := st.GetCharmIDByApplicationName(c.Context(), "foo1")
	c.Assert(err, tc.ErrorIsNil)

	want := []application.ExportApplication{
		{
			UUID:      id0,
			CharmUUID: charmID0,
			Name:      "foo0",
			Life:      life.Dying,
			CharmLocator: charm.CharmLocator{
				Name:     "foo0",
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			Subordinate: false,
			EndpointBindings: map[string]network.SpaceUUID{
				"":          network.AlphaSpaceId,
				"endpoint":  network.AlphaSpaceId,
				"extra":     network.AlphaSpaceId,
				"misc":      network.AlphaSpaceId,
				"juju-info": network.AlphaSpaceId,
			},
		},
		{
			UUID:      id1,
			CharmUUID: charmID1,
			Name:      "foo1",
			Life:      life.Dead,
			CharmLocator: charm.CharmLocator{
				Name:     "foo1",
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			Subordinate: false,
			EndpointBindings: map[string]network.SpaceUUID{
				"":          network.AlphaSpaceId,
				"endpoint":  network.AlphaSpaceId,
				"extra":     network.AlphaSpaceId,
				"misc":      network.AlphaSpaceId,
				"juju-info": network.AlphaSpaceId,
			},
		},
	}

	apps, err := st.GetApplicationsForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apps, tc.DeepEquals, want)
}

func (s *migrationStateSuite) TestGetApplicationsForExportWithNoApplications(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	apps, err := st.GetApplicationsForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apps, tc.HasLen, 0)
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExport(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createIAASApplication(c, "foo", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: "foo/0",
			Password: &application.PasswordInfo{
				PasswordHash:  "password",
				HashAlgorithm: 0,
			},
		},
	})

	unitUUID, err := st.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	units, err := st.GetApplicationUnitsForExport(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, []application.ExportUnit{
		{
			UUID:    unitUUID,
			Name:    "foo/0",
			Machine: machine.Name("0"),
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportMultipleApplications(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createIAASApplication(c, "foo", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: "foo/0",
			Password: &application.PasswordInfo{
				PasswordHash:  "password",
				HashAlgorithm: 0,
			},
		},
	})
	s.createIAASApplication(c, "bar", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: "bar/0",
		},
	})

	unitUUID, err := st.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	units, err := st.GetApplicationUnitsForExport(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, []application.ExportUnit{
		{
			UUID:    unitUUID,
			Name:    "foo/0",
			Machine: machine.Name("0"),
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportSubordinate(c *tc.C) {
	// Arrange:
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	subName := coreunit.Name("foo/0")
	principalName := coreunit.Name("principal/0")
	id := s.createIAASApplication(c, "foo", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: subName,
			Password: &application.PasswordInfo{
				PasswordHash:  "password",
				HashAlgorithm: 0,
			},
		},
	})
	s.createIAASApplication(c, "principal", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: principalName,
		},
	})

	principalUUID, err := st.GetUnitUUIDByName(c.Context(), principalName)
	c.Assert(err, tc.ErrorIsNil)
	subUUID, err := st.GetUnitUUIDByName(c.Context(), subName)
	c.Assert(err, tc.ErrorIsNil)
	s.insertUnitPrincipal(c, principalUUID, subUUID)

	// Act:
	units, err := st.GetApplicationUnitsForExport(c.Context(), id)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, []application.ExportUnit{
		{
			UUID:      subUUID,
			Name:      subName,
			Machine:   "0",
			Principal: principalName,
		},
	})
}

func (s *migrationStateSuite) insertUnitPrincipal(c *tc.C, pUUID, sUUID coreunit.UUID) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO unit_principal (principal_uuid, unit_uuid) VALUES (?,?)`, pUUID, sUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportNoUnits(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createIAASApplication(c, "foo", life.Alive)

	units, err := st.GetApplicationUnitsForExport(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, []application.ExportUnit{})
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportDying(c *tc.C) {
	// We shouldn't export units that are in the dying state, but the old code
	// doesn't prohibit this.

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createIAASApplication(c, "foo", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: "foo/0",
			Password: &application.PasswordInfo{
				PasswordHash:  "password",
				HashAlgorithm: 0,
			},
		},
	})

	unitUUID, err := st.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE uuid = ?", life.Dying, unitUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	units, err := st.GetApplicationUnitsForExport(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, []application.ExportUnit{
		{
			UUID:    unitUUID,
			Name:    "foo/0",
			Machine: machine.Name("0"),
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationUnitsForExportDead(c *tc.C) {
	// We shouldn't export units that are in the dead state, but the old code
	// doesn't prohibit this.

	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createIAASApplication(c, "foo", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: "foo/0",
			Password: &application.PasswordInfo{
				PasswordHash:  "password",
				HashAlgorithm: 0,
			},
		},
	})

	unitUUID, err := st.GetUnitUUIDByName(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE uuid = ?", life.Dead, unitUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	units, err := st.GetApplicationUnitsForExport(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, []application.ExportUnit{
		{
			UUID:    unitUUID,
			Name:    "foo/0",
			Machine: machine.Name("0"),
		},
	})
}

func (s *migrationStateSuite) TestGetApplicationsForExportEndpointBindings(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createIAASApplication(c, "foo", life.Alive)
	charmID, err := st.GetCharmIDByApplicationName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	spaceUUID1 := s.addSpace(c, "beta")
	spaceUUID2 := s.addSpace(c, "gamma")
	s.updateApplicationEndpoint(c, "endpoint", spaceUUID1)
	s.updateApplicationEndpoint(c, "misc", spaceUUID2)

	apps, err := st.GetApplicationsForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apps, tc.DeepEquals, []application.ExportApplication{
		{
			UUID:      id,
			CharmUUID: charmID,
			Name:      "foo",
			Life:      life.Alive,
			CharmLocator: charm.CharmLocator{
				Name:     "foo",
				Revision: 42,
				Source:   charm.CharmHubSource,
			},
			Subordinate: false,
			EndpointBindings: map[string]network.SpaceUUID{
				"":          network.AlphaSpaceId,
				"endpoint":  spaceUUID1,
				"misc":      spaceUUID2,
				"extra":     network.AlphaSpaceId,
				"juju-info": network.AlphaSpaceId,
			},
		},
	})
}

func (s *migrationStateSuite) TestInsertMigratingApplication(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	ctx := c.Context()
	args := application.InsertApplicationArgs{
		Platform: platform,
		Charm: charm.Charm{
			Metadata:      s.minimalMetadata(c, "666"),
			Manifest:      s.minimalManifest(c),
			Source:        charm.CharmHubSource,
			ReferenceName: "666",
			Revision:      42,
			Architecture:  architecture.ARM64,
		},
		Scale:   1,
		Channel: channel,
		Config: map[string]application.ApplicationConfig{
			"foo": {
				Value: "bar",
				Type:  charm.OptionString,
			},
		},
		Settings: application.ApplicationSettings{
			Trust: true,
		},
	}
	id, err := st.InsertMigratingApplication(ctx, "666", args)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("Failed to create application: %s", errors.ErrorStack(err)))
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)
	s.assertDownloadProvenance(c, id, charm.ProvenanceMigration)

	// Ensure that config is empty and trust is false.
	config, settings, err := st.GetApplicationConfigAndSettings(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Value: "bar",
			Type:  charm.OptionString,
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{Trust: true})
}

func (s *migrationStateSuite) TestInsertMigratingApplicationPeerRelations(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := deployment.Platform{
		Channel:      "666",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "risk",
		Branch: "branch",
	}
	ctx := c.Context()
	meta := s.minimalMetadataWithPeerRelation(c, "666", "castor", "pollux")
	meta.Provides = map[string]charm.Relation{
		"no-relation": {
			Name:  "no-relation",
			Role:  charm.RoleProvider,
			Scope: charm.ScopeGlobal,
		},
	}
	args := application.InsertApplicationArgs{
		Platform: platform,
		Charm: charm.Charm{
			Metadata:      meta,
			Manifest:      s.minimalManifest(c),
			Source:        charm.CharmHubSource,
			ReferenceName: "666",
			Revision:      42,
			Architecture:  architecture.ARM64,
		},
		Scale:   1,
		Channel: channel,
		PeerRelations: map[string]int{
			"pollux": 7,
			"castor": 4,
		},
	}
	_, err := st.InsertMigratingApplication(ctx, "666", args)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("Failed to create application: %s", errors.ErrorStack(err)))
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)
	s.assertPeerRelation(c, "666", map[string]int{"pollux": 7, "castor": 4})
	s.assertNoRelationEndpoint(c, "666", "no-relation")
}

func (s *migrationStateSuite) assertNoRelationEndpoint(c *tc.C, appName, endpointName string) {
	values := []string{}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT v.relation_endpoint_uuid
FROM   v_relation_endpoint AS v
WHERE  v.application_name = ?
AND    v.endpoint_name = ?
`, appName, endpointName)

		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return errors.Capture(err)
			}
			values = append(values, value)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(values, tc.DeepEquals, []string{}, tc.Commentf("found relation_endpoint %q", strings.Join(values, ", ")))
}

// addSpace ensures a space with the given name exists in the database,
// creating it if necessary, and returns its name.
func (s *migrationStateSuite) addSpace(c *tc.C, name string) network.SpaceUUID {
	spaceUUID := networktesting.GenSpaceUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO space (uuid, name)
VALUES (?, ?)`, spaceUUID, name)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return spaceUUID
}

func (s *migrationStateSuite) updateApplicationEndpoint(c *tc.C, endpoint, spaceUUID network.SpaceUUID) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var charmRelationUUID string
		err := tx.QueryRowContext(ctx, `
SELECT uuid 
FROM   charm_relation 
WHERE  name = ?
`, endpoint).Scan(&charmRelationUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
UPDATE application_endpoint
SET    space_uuid = ?
WHERE  charm_relation_uuid = ?
`, spaceUUID, charmRelationUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationStateSuite) assertDownloadProvenance(c *tc.C, appID coreapplication.ID, expectedProvenance charm.Provenance) {
	var obtainedProvenance string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT v.provenance
FROM   v_application_charm_download_info AS v
WHERE  v.application_uuid=?
`, appID).Scan(&obtainedProvenance)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedProvenance, tc.Equals, string(expectedProvenance))
}

func (s *unitStateSuite) TestInsertMigratingIAASUnits(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, _, _, err := machinestate.CreateMachine(c.Context(), tx, s.state, clock.WallClock, domainmachine.CreateMachineArgs{
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Architecture: architecture.ARM64,
			},
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.InsertMigratingIAASUnits(c.Context(), appID, application.ImportUnitArg{
		UnitName: "foo/666",
		Machine:  "0",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, appID)
}

func (s *unitStateSuite) TestInsertMigratingCAASUnits(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingCAASUnits(c.Context(), appID, application.ImportUnitArg{
		UnitName: "foo/666",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, appID)
}

func (s *unitStateSuite) TestInsertMigratingCAASUnitsSubordinate(c *tc.C) {
	principal := unittesting.GenNewName(c, "bar/0")
	sub := unittesting.GenNewName(c, "foo/666")
	s.createIAASApplication(c, "bar", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: principal,
		},
	})
	subAppID := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingCAASUnits(c.Context(), subAppID, application.ImportUnitArg{
		UnitName:  sub,
		Principal: principal,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, subAppID)
	s.assertUnitPrincipal(c, principal, sub)
}

func (s *unitStateSuite) TestInsertMigratingIAASUnitsSubordinate(c *tc.C) {
	principal := unittesting.GenNewName(c, "bar/0")
	sub := unittesting.GenNewName(c, "foo/666")
	s.createIAASApplication(c, "bar", life.Alive, application.InsertIAASUnitArg{
		InsertUnitArg: application.InsertUnitArg{
			UnitName: principal,
		},
	})
	subAppID := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingIAASUnits(c.Context(), subAppID, application.ImportUnitArg{
		UnitName:  "foo/666",
		Machine:   "0",
		Principal: principal,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, subAppID)
	s.assertUnitPrincipal(c, principal, sub)
}

func (s *unitStateSuite) assertInsertMigratingUnits(c *tc.C, appID coreapplication.ID) {
	var unitName string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM unit WHERE application_uuid=?", appID).Scan(&unitName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitName, tc.Equals, "foo/666")
}
