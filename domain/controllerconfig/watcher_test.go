// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerconfig

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatch(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "controller_config")

	svc := service.NewWatchableService(state.NewState(func() (database.TxnRunner, error) { return factory() }),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.Watch()
	c.Assert(err, jc.ErrorIsNil)

	w := watchertest.NewStringsWatcherC(c, watcher)

	// Wait for the initial change.
	w.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	cfgMap := map[string]any{
		controller.AuditingEnabled:        true,
		controller.AuditLogCaptureArgs:    false,
		controller.AuditLogMaxBackups:     10,
		controller.APIPortOpenDelay:       "100ms",
		controller.MigrationMinionWaitMax: "101ms",
	}

	err = svc.UpdateControllerConfig(context.Background(), cfgMap, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertChangeStreamIdle(c)

	// Get the change.
	w.AssertChange(
		controller.AuditingEnabled,
		controller.AuditLogCaptureArgs,
		controller.AuditLogMaxBackups,
		controller.APIPortOpenDelay,
		controller.MigrationMinionWaitMax,
	)
}
