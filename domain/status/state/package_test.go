// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type baseSuite struct {
	schematesting.ModelSuite
}

func (s *baseSuite) workloadStatus(now time.Time) *status.StatusInfo[status.WorkloadStatusType] {
	return &status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(now),
	}
}

func (s *baseSuite) createCAASUnitArg(c *tc.C) application.AddCAASUnitArg {
	return application.AddCAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
		},
	}
}

func (s *baseSuite) createIAASUnitArg(c *tc.C) application.AddIAASUnitArg {
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	return application.AddIAASUnitArg{
		MachineNetNodeUUID: netNodeUUID,
		MachineUUID:        tc.Must(c, coremachine.NewUUID),
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: netNodeUUID,
		},
	}
}

func (s *baseSuite) createIAASApplicationWithNUnits(
	c *tc.C,
	name string,
	nUnits int,
) (coreapplication.UUID, []coreunit.UUID) {
	units := make([]application.AddIAASUnitArg, nUnits)
	for i := range units {
		netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
		units[i].MachineUUID = tc.Must(c, coremachine.NewUUID)
		units[i].MachineNetNodeUUID = netNodeUUID
		units[i].NetNodeUUID = netNodeUUID
	}

	return s.createIAASApplication(
		c, name, life.Alive, false, s.workloadStatus(time.Now()), units...,
	)
}

func (s *baseSuite) createIAASApplication(
	c *tc.C,
	name string,
	l life.Life,
	subordinate bool,
	appStatus *status.StatusInfo[status.WorkloadStatusType],
	units ...application.AddIAASUnitArg,
) (coreapplication.UUID, []coreunit.UUID) {
	appState := applicationstate.NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)

	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	appID, _, err := appState.CreateIAASApplication(ctx, name, application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name:        name,
					Subordinate: subordinate,
					Provides: map[string]charm.Relation{
						"endpoint": {
							Name:  "endpoint",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
						"misc": {
							Name:  "misc",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
					},
				},
				Manifest:      s.minimalManifest(c),
				ReferenceName: name,
				Source:        charm.CharmHubSource,
				Revision:      42,
				Hash:          "hash",
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident",
				DownloadURL:        "https://example.com",
				DownloadSize:       42,
			},
			Status: appStatus,
		},
	}, units)
	c.Assert(err, tc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, 0, len(units))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE application_uuid = ?", l, appID)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(
			ctx,
			"SELECT uuid FROM unit WHERE application_uuid = ?",
			appID.String(),
		)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var unitUUID string
			c.Assert(rows.Scan(&unitUUID), tc.ErrorIsNil)
			unitUUIDs = append(unitUUIDs, coreunit.UUID(unitUUID))
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return appID, unitUUIDs
}

func (s *baseSuite) createCAASApplication(
	c *tc.C,
	name string,
	l life.Life,
	subordinate bool,
	appStatus *status.StatusInfo[status.WorkloadStatusType],
	units ...application.AddCAASUnitArg,
) (coreapplication.UUID, []coreunit.UUID) {
	appState := applicationstate.NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)

	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	appID, err := appState.CreateCAASApplication(ctx, name, application.AddCAASApplicationArg{
		Scale: len(units),
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name:        name,
					Subordinate: subordinate,
					Provides: map[string]charm.Relation{
						"endpoint": {
							Name:  "endpoint",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
						"misc": {
							Name:  "misc",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
					},
				},
				Manifest:      s.minimalManifest(c),
				ReferenceName: name,
				Source:        charm.CharmHubSource,
				Revision:      42,
				Hash:          "hash",
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident",
				DownloadURL:        "https://example.com",
				DownloadSize:       42,
			},
			Status: appStatus,
		},
	}, units)
	c.Assert(err, tc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, 0, len(units))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE application_uuid = ?", l, appID)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(
			ctx,
			"SELECT uuid FROM unit WHERE application_uuid = ?",
			appID.String(),
		)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var unitUUID string
			c.Assert(rows.Scan(&unitUUID), tc.ErrorIsNil)
			unitUUIDs = append(unitUUIDs, coreunit.UUID(unitUUID))
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return appID, unitUUIDs
}

func (s *baseSuite) insertRemoteApplication(c *tc.C, appUUID, remoteAppUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_remote_offerer (uuid, life_id, application_uuid, offer_uuid, version, offerer_model_uuid, macaroon)
VALUES (?, 0, ?, "1", "2", "3", "4")
`, remoteAppUUID, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) setApplicationLXDProfile(c *tc.C, appUUID coreapplication.UUID, profile string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET lxd_profile = ? WHERE uuid = (SELECT charm_uuid FROM application WHERE uuid = ?)
`, profile, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) setApplicationSubordinate(c *tc.C, principal coreunit.UUID, subordinate coreunit.UUID) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?, ?);`, subordinate, principal)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) minimalManifest(c *tc.C) charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}
