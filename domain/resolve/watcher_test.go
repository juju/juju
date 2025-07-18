// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolve_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	"github.com/juju/juju/domain/resolve/service"
	"github.com/juju/juju/domain/resolve/state"
	"github.com/juju/juju/domain/status"
	statusstate "github.com/juju/juju/domain/status/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchUnitResolveModeNotFound(c *tc.C) {
	svc := s.setupService(c)

	_, err := svc.WatchUnitResolveMode(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *watcherSuite) TestWatchUnitResoloveMode(c *tc.C) {
	u1 := application.AddIAASUnitArg{}
	u2 := application.AddIAASUnitArg{}
	s.createApplication(c, "foo", u1, u2)

	svc := s.setupService(c)

	watcher, err := svc.WatchUnitResolveMode(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that nothing changes if nothing happens (pre-test).
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that a notification is emitted if a unit is resolved
	harness.AddTest(c, func(c *tc.C) {
		err := svc.ResolveUnit(c.Context(), "foo/0", resolve.ResolveModeRetryHooks)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a notification is emitted if a unit resolve mode is changed
	harness.AddTest(c, func(c *tc.C) {
		err := svc.ResolveUnit(c.Context(), "foo/0", resolve.ResolveModeNoHooks)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a notification is emitted if a unit resolve mode is cleared
	harness.AddTest(c, func(c *tc.C) {
		err := svc.ClearResolved(c.Context(), "foo/0")
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens (post-test).
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert no notification is emitted if another unit is resolved
	harness.AddTest(c, func(c *tc.C) {
		err := svc.ResolveUnit(c.Context(), "foo/1", resolve.ResolveModeRetryHooks)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C) *service.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_resolved")

	return service.NewWatchableService(
		state.NewState(modelDB),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}

func (s *watcherSuite) createApplication(c *tc.C, name string, units ...application.AddIAASUnitArg) []coreunit.UUID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	statusSt := statusstate.NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

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
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident",
				DownloadURL:        "https://example.com",
				DownloadSize:       42,
			},
		},
		Scale: len(units),
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	unitNames, _, err := appState.AddIAASUnits(ctx, appID, units...)
	c.Assert(err, tc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, len(units))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for i, unitName := range unitNames {
			var uuid coreunit.UUID
			err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&uuid)
			if err != nil {
				return err
			}
			unitUUIDs[i] = uuid
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Put the units in error status so they can be resolved
	for _, u := range unitUUIDs {
		err := statusSt.SetUnitAgentStatus(ctx, u, status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusError,
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	return unitUUIDs
}

func (s *watcherSuite) minimalManifest(c *tc.C) charm.Manifest {
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
