// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
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
			EndpointBindings: map[string]string{
				"":          network.AlphaSpaceId,
				"endpoint":  "",
				"extra":     "",
				"juju-info": "",
				"misc":      "",
			},
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
			EndpointBindings: map[string]string{
				"":          network.AlphaSpaceId,
				"endpoint":  "",
				"extra":     "",
				"juju-info": "",
				"misc":      "",
			},
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
			EndpointBindings: map[string]string{
				"":          network.AlphaSpaceId,
				"endpoint":  "",
				"extra":     "",
				"misc":      "",
				"juju-info": "",
			},
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
			EndpointBindings: map[string]string{
				"":          network.AlphaSpaceId,
				"endpoint":  "",
				"extra":     "",
				"misc":      "",
				"juju-info": "",
			},
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

func (s *migrationStateSuite) TestGetApplicationUnitsForExportMultipleApplications(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
		Password: &application.PasswordInfo{
			PasswordHash:  "password",
			HashAlgorithm: 0,
		},
	})
	s.createApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: "bar/0",
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

func (s *migrationStateSuite) TestGetApplicationUnitsForExportSubordinate(c *gc.C) {
	// Arrange:
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	subName := coreunit.Name("foo/0")
	principalName := coreunit.Name("principal/0")
	id := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: subName,
		Password: &application.PasswordInfo{
			PasswordHash:  "password",
			HashAlgorithm: 0,
		},
	})
	s.createApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: principalName,
	})

	principalUUID, err := st.GetUnitUUIDByName(context.Background(), principalName)
	c.Assert(err, jc.ErrorIsNil)
	subUUID, err := st.GetUnitUUIDByName(context.Background(), subName)
	c.Assert(err, jc.ErrorIsNil)
	s.insertUnitPrincipal(c, principalUUID, subUUID)

	// Act:
	units, err := st.GetApplicationUnitsForExport(context.Background(), id)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(units, jc.DeepEquals, []application.ExportUnit{
		{
			UUID:      subUUID,
			Name:      "foo/0",
			Machine:   "0",
			Principal: principalName,
		},
	})
}

func (s *migrationStateSuite) insertUnitPrincipal(c *gc.C, pUUID, sUUID coreunit.UUID) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO unit_principal (principal_uuid, unit_uuid) VALUES (?,?)`, pUUID, sUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
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

func (s *migrationStateSuite) TestGetApplicationsForExportEndpointBindings(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	id := s.createApplication(c, "foo", life.Alive)
	charmID, err := st.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	spaceUUID1 := s.addSpace(c, "beta")
	spaceUUID2 := s.addSpace(c, "gamma")
	s.updateApplicationEndpoint(c, "endpoint", spaceUUID1)
	s.updateApplicationEndpoint(c, "misc", spaceUUID2)

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
			EndpointBindings: map[string]string{
				"":          network.AlphaSpaceId,
				"endpoint":  spaceUUID1,
				"misc":      spaceUUID2,
				"extra":     "",
				"juju-info": "",
			},
		},
	})
}

func (s *migrationStateSuite) TestInsertMigratingApplication(c *gc.C) {
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
	ctx := context.Background()
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
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Failed to create application: %s", errors.ErrorStack(err)))
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)
	s.assertDownloadProvenance(c, id, charm.ProvenanceMigration)

	// Ensure that config is empty and trust is false.
	config, settings, err := st.GetApplicationConfigAndSettings(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.DeepEquals, map[string]application.ApplicationConfig{
		"foo": {
			Value: "bar",
			Type:  charm.OptionString,
		},
	})
	c.Check(settings, gc.DeepEquals, application.ApplicationSettings{Trust: true})
}

func (s *migrationStateSuite) TestInsertMigratingApplicationPeerRelations(c *gc.C) {
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
	ctx := context.Background()
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
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Failed to create application: %s", errors.ErrorStack(err)))
	scale := application.ScaleState{Scale: 1}
	s.assertApplication(c, "666", platform, channel, scale, false)
	s.assertPeerRelation(c, "666", map[string]int{"pollux": 7, "castor": 4})
	s.assertNoRelationEndpoint(c, "666", "no-relation")
}

func (s *migrationStateSuite) assertNoRelationEndpoint(c *gc.C, appName, endpointName string) {
	values := []string{}
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(values, jc.DeepEquals, []string{}, gc.Commentf("found relation_endpoint %q", strings.Join(values, ", ")))
}

// addSpace ensures a space with the given name exists in the database,
// creating it if necessary, and returns its name.
func (s *migrationStateSuite) addSpace(c *gc.C, name string) string {
	spaceUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO space (uuid, name)
VALUES (?, ?)`, spaceUUID, name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return spaceUUID
}

func (s *migrationStateSuite) updateApplicationEndpoint(c *gc.C, endpoint, space_uuid string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
`, space_uuid, charmRelationUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *migrationStateSuite) assertDownloadProvenance(c *gc.C, appID coreapplication.ID, expectedProvenance charm.Provenance) {
	var obtainedProvenance string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedProvenance, gc.Equals, string(expectedProvenance))
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

func (s *unitStateSubordinateSuite) TestInsertMigratingCAASUnitsSubordinate(c *gc.C) {
	principal := unittesting.GenNewName(c, "bar/0")
	sub := unittesting.GenNewName(c, "foo/666")
	s.createApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: principal,
	})
	subAppID := s.createApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingCAASUnits(context.Background(), subAppID, application.ImportUnitArg{
		UnitName:  sub,
		Principal: principal,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, subAppID)
	s.assertUnitPrincipal(c, principal, sub)
}

func (s *unitStateSubordinateSuite) TestInsertMigratingIAASUnitsSubordinate(c *gc.C) {
	principal := unittesting.GenNewName(c, "bar/0")
	sub := unittesting.GenNewName(c, "foo/666")
	s.createApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: principal,
	})
	subAppID := s.createApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingIAASUnits(context.Background(), subAppID, application.ImportUnitArg{
		UnitName:  "foo/666",
		Machine:   "0",
		Principal: principal,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, subAppID)
	s.assertUnitPrincipal(c, principal, sub)
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
