// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllernode_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/controllernode/service"
	"github.com/juju/juju/domain/controllernode/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
}

func (s *watcherSuite) TestControllerNodes(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "controller_node")

	ctx := c.Context()
	svc := s.setupService(c, factory)
	watcher, err := svc.WatchControllerNodes(ctx)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that we get the controller node created event.
	harness.AddTest(func(c *tc.C) {
		svc.CurateNodes(ctx, []string{"controller0"}, nil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get the second and third controller nodes created event.
	harness.AddTest(func(c *tc.C) {
		svc.CurateNodes(ctx, []string{"controller1", "controller2"}, nil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get the removed controllers event.
	harness.AddTest(func(c *tc.C) {
		svc.CurateNodes(ctx, nil, []string{"controller1", "controller2"})
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Nothing happens so no change.
	harness.AddTest(func(c *tc.C) {
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}

	return service.NewWatchableService(
		state.NewState(modelDB),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}
