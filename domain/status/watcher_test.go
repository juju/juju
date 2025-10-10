// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	context "context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/domain/status/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod",  "iaas", "test-model", "ec2")
		`, modelUUID.String(), testing.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchOfferStatus(c *tc.C) {
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive)
	offerUUID := s.createOffer(c, appUUID, "endpoint")

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "status")
	svc := s.setupService(c, factory)

	watcher, err := svc.WatchOfferStatus(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that setting the status triggers the watcher.

	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetApplicationStatus(c.Context(), "foo", status.StatusInfo{
			Status:  status.Active,
			Message: "it's active!",
			Data:    map[string]any{"foo": "bar"},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that we get no notifications when the status is set but not changed.

	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetApplicationStatus(c.Context(), "foo", status.StatusInfo{
			Status:  status.Active,
			Message: "it's active!",
			Data:    map[string]any{"foo": "bar"},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that changing the status triggers the watcher.

	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetApplicationStatus(c.Context(), "foo", status.StatusInfo{
			Status:  status.Blocked,
			Message: "it's blocked!",
			Data:    map[string]any{"bar": "foo"},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchOfferStatusApplicationWithUnits(c *tc.C) {
	units := make([]application.AddIAASUnitArg, 3)
	for i := range units {
		netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
		units[i].MachineUUID = tc.Must(c, coremachine.NewUUID)
		units[i].MachineNetNodeUUID = netNodeUUID
		units[i].NetNodeUUID = netNodeUUID
		units[i].WorkloadStatus = &domainstatus.StatusInfo[domainstatus.WorkloadStatusType]{
			Status: domainstatus.WorkloadStatusActive,
		}
		units[i].AgentStatus = &domainstatus.StatusInfo[domainstatus.UnitAgentStatusType]{
			Status: domainstatus.UnitAgentStatusAllocating,
		}
	}
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive, units...)
	offerUUID := s.createOffer(c, appUUID, "endpoint")

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "status")
	svc := s.setupService(c, factory)

	watcher, err := svc.WatchOfferStatus(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that setting the status of a unit triggers the watcher.

	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetUnitWorkloadStatus(c.Context(), "foo/0", status.StatusInfo{
			Status:  status.Maintenance,
			Message: "it's active!",
			Data:    map[string]any{"foo": "bar"},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that  setting the status of a unit, that does not cascade to the
	// application, does not trigger the watcher

	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetUnitWorkloadStatus(c.Context(), "foo/1", status.StatusInfo{
			Status:  status.Waiting,
			Message: "it's waiting!",
			Data:    map[string]any{"foo": "bar"},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that setting a unit agent status to error does trigger the watcher
	// NOTE: error status for an agent is a special case, all other agent
	// statuses are ignored when deriving the application status.

	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetUnitAgentStatus(c.Context(), "foo/2", status.StatusInfo{
			Status:  status.Error,
			Message: "it's an error!",
			Data:    map[string]any{"foo": "bar"},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchOfferStatusOfferDeleted(c *tc.C) {
	appUUID, _ := s.createIAASApplication(c, "foo", life.Alive)
	offerUUID := s.createOffer(c, appUUID, "endpoint")

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "status")
	svc := s.setupService(c, factory)

	watcher, err := svc.WatchOfferStatus(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that deleting the offer triggers the watcher.

	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `DELETE FROM offer_endpoint WHERE offer_uuid = ?`, offerUUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM offer WHERE uuid = ?`, offerUUID); err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchOfferNotFound(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "status")
	svc := s.setupService(c, factory)

	offerUUID := tc.Must(c, offer.NewUUID)
	_, err := svc.WatchOfferStatus(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewWatchableService(
		state.NewModelState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		nil,
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		nil,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) createIAASApplication(
	c *tc.C,
	name string,
	l life.Life,
	units ...application.AddIAASUnitArg,
) (coreapplication.UUID, []coreunit.UUID) {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	appState := applicationstate.NewState(
		modelDB, clock.WallClock, loggertesting.WrapCheckLog(c),
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
				Manifest:      s.minimalManifest(),
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
		},
	}, units)
	c.Assert(err, tc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, 0, len(units))
	err = s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

func (s *watcherSuite) minimalManifest() charm.Manifest {
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

func (s *watcherSuite) createOffer(c *tc.C, appUUID coreapplication.UUID, endpointName string) offer.UUID {
	offerUUID := tc.Must(c, offer.NewUUID)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var endpointUUID string
		err := tx.QueryRowContext(ctx, `
SELECT ae.uuid
FROM   application_endpoint AS ae
JOIN   charm_relation AS cr ON ae.charm_relation_uuid = cr.uuid
WHERE  ae.application_uuid = ? AND cr.name = ?
`, appUUID, endpointName).Scan(&endpointUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO offer (uuid, name) VALUES (?, ?)`, offerUUID, endpointName)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO offer_endpoint (offer_uuid, endpoint_uuid) VALUES (?, ?)`, offerUUID, endpointUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return offerUUID
}
