// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/crossmodel"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ModelSuite

	now time.Time
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.now = time.Now().UTC()
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
		c, name, life.Alive, s.workloadStatus(time.Now()), units...,
	)
}

func (s *baseSuite) createSubordinateIAASApplication(
	c *tc.C,
	name string,
	l life.Life,
	appStatus *status.StatusInfo[status.WorkloadStatusType],
	units ...application.AddIAASUnitArg,
) (coreapplication.UUID, []coreunit.UUID) {
	ch := charm.Charm{
		Metadata: charm.Metadata{
			Name:        name,
			Subordinate: true,
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
	}
	return s.createIAASApplicationWithCharm(c, name, l, ch, appStatus, units...)
}

func (s *baseSuite) createIAASApplication(
	c *tc.C,
	name string,
	l life.Life,
	appStatus *status.StatusInfo[status.WorkloadStatusType],
	units ...application.AddIAASUnitArg,
) (coreapplication.UUID, []coreunit.UUID) {
	ch := charm.Charm{
		Metadata: charm.Metadata{
			Name: name,
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
	}
	return s.createIAASApplicationWithCharm(c, name, l, ch, appStatus, units...)
}

func (s *baseSuite) createIAASRemoteApplicationOfferer(
	c *tc.C,
	name string,
) (coreapplication.UUID, coreremoteapplication.UUID) {
	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)

	ch := charm.Charm{
		Metadata: charm.Metadata{
			Name: name,
			Provides: map[string]charm.Relation{
				"endpoint": {
					Name:      "endpoint",
					Interface: "interf",
					Role:      charm.RoleProvider,
					Scope:     charm.ScopeGlobal,
				},
				"misc": {
					Name:      "misc",
					Interface: "interf",
					Role:      charm.RoleProvider,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
		Manifest:      s.minimalManifest(c),
		ReferenceName: name,
		Source:        charm.CMRSource,
		Revision:      42,
		Hash:          "hash",
		Architecture:  architecture.ARM64,
	}

	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	err := cmrState.AddRemoteApplicationOfferer(c.Context(), name, crossmodelrelation.AddRemoteApplicationOffererArgs{
		RemoteApplicationUUID: remoteAppUUID.String(),
		ApplicationUUID:       appUUID.String(),
		CharmUUID:             tc.Must(c, uuid.NewUUID).String(),
		Charm:                 ch,
		OfferUUID:             tc.Must(c, uuid.NewUUID).String(),
		OfferURL:              tc.Must1(c, crossmodel.ParseOfferURL, fmt.Sprintf("controller:qualifier/model.%s", name)).String(),
		OffererModelUUID:      tc.Must(c, uuid.NewUUID).String(),
		EncodedMacaroon:       []byte("macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, remoteAppUUID
}

func (s *baseSuite) createIAASApplicationWithCharm(
	c *tc.C,
	name string,
	l life.Life,
	ch charm.Charm,
	appStatus *status.StatusInfo[status.WorkloadStatusType],
	units ...application.AddIAASUnitArg,
) (coreapplication.UUID, []coreunit.UUID) {
	appState := applicationstate.NewState(
		s.TxnRunnerFactory(), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
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
			Charm:    ch,
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
	appStatus *status.StatusInfo[status.WorkloadStatusType],
	units ...application.AddCAASUnitArg,
) (coreapplication.UUID, []coreunit.UUID) {
	appState := applicationstate.NewState(
		s.TxnRunnerFactory(), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
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
					Name: name,
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

// addRelationWithLifeAndID inserts a new relation into the database with the
// given details.
func (s *baseSuite) addRelationWithLifeAndID(c *tc.C, life corelife.Value, relationID int) corerelation.UUID {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation (uuid, relation_id, life_id, scope_id)
SELECT ?, ?, id, 0
FROM life
WHERE value = ?
`, relationUUID, relationID, life)
		return err
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to insert relation %s, id %d", relationUUID, relationID))
	return relationUUID
}

func (s *baseSuite) addRelationToApplication(c *tc.C, appUUID coreapplication.UUID, relationUUID corerelation.UUID) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var charmRelationUUID string
		err := tx.QueryRowContext(ctx, `SELECT uuid FROM charm_relation WHERE name = 'endpoint'`).Scan(&charmRelationUUID)
		if err != nil {
			return err
		}

		var endpointUUID string
		err = tx.QueryRowContext(ctx, `SELECT uuid FROM application_endpoint WHERE application_uuid = ? AND charm_relation_uuid = ?`, appUUID, charmRelationUUID).Scan(&endpointUUID)
		if err != nil {
			return err
		}

		relationEndpointUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, `INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?);`, relationEndpointUUID, relationUUID, endpointUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO application_exposed_endpoint_cidr (application_uuid, application_endpoint_uuid, cidr) VALUES (?, ?, "10.0.0.0/24") ON CONFLICT DO NOTHING;`, appUUID, endpointUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}
