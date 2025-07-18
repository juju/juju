// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerconfig

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/domain/controllerconfig/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchControllerConfig(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "controller_config")

	svc := service.NewWatchableService(state.NewState(func() (database.TxnRunner, error) { return factory() }),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.WatchControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		cfgMap := map[string]any{
			controller.AuditingEnabled:        true,
			controller.AuditLogCaptureArgs:    false,
			controller.AuditLogMaxBackups:     10,
			controller.MigrationMinionWaitMax: "101ms",
		}

		err = svc.UpdateControllerConfig(c.Context(), cfgMap, nil)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.Check(
			watchertest.StringSliceAssert[string](
				controller.AuditingEnabled,
				controller.AuditLogCaptureArgs,
				controller.AuditLogMaxBackups,
				controller.MigrationMinionWaitMax,
			),
		)
	})

	harness.AddTest(c, func(c *tc.C) {
		cfgMap := map[string]any{
			controller.AuditLogMaxBackups: 11,
		}

		err = svc.UpdateControllerConfig(c.Context(), cfgMap, nil)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.Check(
			watchertest.StringSliceAssert[string](
				controller.AuditLogMaxBackups,
			),
		)
	})

	harness.AddTest(c, func(c *tc.C) {
		cfgMap := map[string]any{
			controller.AuditLogMaxBackups: 11,
		}

		err = svc.UpdateControllerConfig(c.Context(), cfgMap, nil)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// The value is the same, we shouldn't get a change.
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}
