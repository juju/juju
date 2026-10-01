// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllernode_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

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

func TestWatcherSuite(t *testing.T) {
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
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that we get the controller node created event.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddDqliteNode(ctx, "0", uint64(1), "10.0.0.1")
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get the update controller node event.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddDqliteNode(ctx, "0", uint64(1), "10.0.0.2")
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get a new controller node.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddDqliteNode(ctx, "0", uint64(2), "10.0.0.3")
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestControllerAPIAddresses(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "controller_api_address")
	svc := s.setupService(c, factory)
	watcher, err := svc.WatchControllerAPIAddresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	// This watcher still observes the legacy table until readers migrate.
	exec := func(c *tc.C, query string) {
		err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, query)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}
	harness.AddTest(c, func(c *tc.C) {
		exec(c, "INSERT INTO controller_api_address (controller_id, address, scope) VALUES ('0', '10.9.9.32:42', 'local-cloud')")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(c, func(c *tc.C) {
		c.Assert(svc.AddDqliteNode(c.Context(), "1", uint64(1), "10.0.0.1"), tc.ErrorIsNil)
		exec(c, "INSERT INTO controller_api_address (controller_id, address, scope) VALUES ('1', '10.9.9.32:42', 'local-cloud')")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(c, func(c *tc.C) {
		exec(c, "UPDATE controller_api_address SET address = '10.43.25.2:42' WHERE controller_id = '0'")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(c, func(c *tc.C) {
		exec(c, "DELETE FROM controller_api_address WHERE controller_id = '0'")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(c, func(c *tc.C) {
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}

	return service.NewWatchableService(
		state.NewState(modelDB),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
	)
}
