// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolve_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchUnitResolveModeNotFound(c *gc.C) {
	svc := s.setupService(c)

	_, err := svc.WatchUnitResolveMode(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *watcherSuite) TestWatchUnitResoloveMode(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	s.createApplication(c, "foo", u1, u2)

	svc := s.setupService(c)

	watcher, err := svc.WatchUnitResolveMode(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that nothing changes if nothing happens (pre-test).
	harness.AddTest(func(c *gc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that a notification is emitted if a unit is resolved
	harness.AddTest(func(c *gc.C) {
		err := svc.ResolveUnit(context.Background(), "foo/0", resolve.ResolveModeRetryHooks)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a notification is emitted if a unit resolve mode is changed
	harness.AddTest(func(c *gc.C) {
		err := svc.ResolveUnit(context.Background(), "foo/0", resolve.ResolveModeNoHooks)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a notification is emitted if a unit resolve mode is cleared
	harness.AddTest(func(c *gc.C) {
		err := svc.ClearResolved(context.Background(), "foo/0")
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens (post-test).
	harness.AddTest(func(c *gc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert no notification is emitted if another unit is resolved
	harness.AddTest(func(c *gc.C) {
		err := svc.ResolveUnit(context.Background(), "foo/1", resolve.ResolveModeRetryHooks)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *gc.C) *service.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_resolved")

	return service.NewWatchableService(
		state.NewState(modelDB),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}

func (s *watcherSuite) createApplication(c *gc.C, name string, units ...application.AddUnitArg) []coreunit.UUID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	statusSt := statusstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

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
	ctx := context.Background()

	appID, err := appState.CreateApplication(ctx, name, application.AddApplicationArg{
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
		Scale: len(units),
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	unitNames, err := appState.AddIAASUnits(ctx, appID, units...)
	c.Assert(err, jc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, len(units))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)

	// Put the units in error status so they can be resolved
	for _, u := range unitUUIDs {
		err := statusSt.SetUnitAgentStatus(ctx, u, status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusError,
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	return unitUUIDs
}

func (s *watcherSuite) minimalManifest(c *gc.C) charm.Manifest {
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
