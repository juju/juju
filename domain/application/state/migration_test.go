// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	machinetesting "github.com/juju/juju/core/machine/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	machinestate "github.com/juju/juju/domain/machine/state"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationStateSuite struct {
	baseSuite
}

func TestMigrationStateSuite(t *testing.T) {
	tc.Run(t, &migrationStateSuite{})
}

func (s *migrationStateSuite) TestInsertMigratingApplication(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), s.modelUUID, clock.WallClock, loggertesting.WrapCheckLog(c))

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
		Config: map[string]application.AddApplicationConfig{
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
			Value: ptr("bar"),
			Type:  charm.OptionString,
		},
	})
	c.Check(settings, tc.DeepEquals, application.ApplicationSettings{Trust: true})
}

func (s *migrationStateSuite) assertDownloadProvenance(c *tc.C, appID coreapplication.UUID, expectedProvenance charm.Provenance) {
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
		_, err := machinestate.CreateMachine(c.Context(), tx, s.state, clock.WallClock, machinestate.CreateMachineArgs{
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Architecture: architecture.ARM64,
			},
			MachineUUID: machinetesting.GenUUID(c).String(),
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.InsertMigratingIAASUnits(c.Context(), appID, application.ImportIAASUnitArg{
		ImportUnitArg: application.ImportUnitArg{
			UnitName: "foo/666",
		},
		Machine: "0",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, appID)
}

func (s *unitStateSuite) TestInsertMigratingCAASUnits(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingCAASUnits(c.Context(), appID, application.ImportCAASUnitArg{
		ImportUnitArg: application.ImportUnitArg{
			UnitName: "foo/666",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, appID)
}

func (s *unitStateSuite) TestInsertMigratingIAASUnitsSubordinate(c *tc.C) {
	sub := unittesting.GenNewName(c, "foo/666")
	_, unitUUIDs := s.createIAASApplicationWithNUnits(c, "bar", life.Alive, 1)
	subAppID := s.createIAASApplication(c, "foo", life.Alive)

	principal, err := s.state.GetUnitNameForUUID(c.Context(), unitUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.InsertMigratingIAASUnits(c.Context(), subAppID, application.ImportIAASUnitArg{
		ImportUnitArg: application.ImportUnitArg{
			UnitName: sub,
		},
		Principal: principal,
		Machine:   "0",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.assertInsertMigratingUnits(c, subAppID)
	s.assertUnitPrincipal(c, unitUUIDs[0], sub)
}

func (s *unitStateSuite) TestInsertMigratingIAASUnitsWithWorkloadVersion(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := machinestate.CreateMachine(c.Context(), tx, s.state, clock.WallClock, machinestate.CreateMachineArgs{
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Architecture: architecture.ARM64,
			},
			MachineUUID: machinetesting.GenUUID(c).String(),
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.InsertMigratingIAASUnits(c.Context(), appID, application.ImportIAASUnitArg{
		ImportUnitArg: application.ImportUnitArg{
			UnitName:        "foo/0",
			WorkloadVersion: "1.2.3",
		},
		Machine: "0",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the workload version was inserted
	version, err := s.state.GetUnitWorkloadVersion(c.Context(), coreunit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version, tc.Equals, "1.2.3")
}

func (s *unitStateSuite) TestInsertMigratingCAASUnitsWithWorkloadVersion(c *tc.C) {
	appID := s.createIAASApplication(c, "foo", life.Alive)

	err := s.state.InsertMigratingCAASUnits(c.Context(), appID, application.ImportCAASUnitArg{
		ImportUnitArg: application.ImportUnitArg{
			UnitName:        "foo/0",
			WorkloadVersion: "2.0.0",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the workload version was inserted
	version, err := s.state.GetUnitWorkloadVersion(c.Context(), coreunit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(version, tc.Equals, "2.0.0")
}

func (s *unitStateSuite) assertInsertMigratingUnits(c *tc.C, appID coreapplication.UUID) {
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
