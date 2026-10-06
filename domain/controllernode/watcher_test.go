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
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/controllernode"
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

func (s *watcherSuite) TestControllerAgentAddresses(c *tc.C) {
	s.checkAddressWatcher(c, "controller_agent_address", (*service.WatchableService).WatchControllerAgentAddresses)
}

func (s *watcherSuite) TestControllerClientAddresses(c *tc.C) {
	s.checkAddressWatcher(c, "controller_client_address", (*service.WatchableService).WatchControllerClientAddresses)
}

func (s *watcherSuite) checkAddressWatcher(c *tc.C, table string, watch func(*service.WatchableService, context.Context) (watcher.NotifyWatcher, error)) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, database.ControllerNS)
	svc := s.setupService(c, factory)
	w, err := watch(svc, c.Context())
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	// Publish through the setter after the initial event, so a consumer that
	// read an empty projection is notified when addresses become available.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetAPIAddresses(c.Context(), controllernode.SetAPIAddressArgs{
			APIAddresses: map[string]network.SpaceHostPorts{
				"0": network.NewSpaceHostPorts(17070, "10.0.0.1"),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Shared endpoints have no controller ID but still notify consumers.
	harness.AddTest(c, func(c *tc.C) {
		s.execAddressSQL(c, "INSERT INTO "+table+" (uuid, address, scope) VALUES ('shared', 'shared.example.com:17070', 'public')")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	for _, update := range []string{
		"scope = 'local-cloud'", "priority = 2", "controller_id = '0'",
		"address = 'updated.example.com:17070'",
	} {
		harness.AddTest(c, func(c *tc.C) {
			s.execAddressSQL(c, "UPDATE "+table+" SET "+update+" WHERE uuid = 'shared'")
		}, func(w watchertest.WatcherC[struct{}]) {
			w.AssertChange()
		})
	}
	harness.AddTest(c, func(c *tc.C) {
		s.execAddressSQL(c, "UPDATE "+table+" SET priority = priority")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	harness.AddTest(c, func(c *tc.C) {
		s.execAddressSQL(c, "DELETE FROM "+table+" WHERE uuid = 'shared'")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Each audience observes only its own projection.
	harness.AddTest(c, func(c *tc.C) {
		for _, other := range []string{"controller_agent_address", "controller_client_address", "controller_peer_address"} {
			if other != table {
				s.execAddressSQL(c, "INSERT INTO "+other+" (uuid, controller_id, address, scope) VALUES ('other', '0', 'other.example.com:17070', 'public')")
			}
		}
		s.execAddressSQL(c, "INSERT INTO controller_api_address (controller_id, address, scope) VALUES ('0', 'legacy.example.com:17070', 'public')")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	harness.Run(c, struct{}{})
}

func (s *watcherSuite) execAddressSQL(c *tc.C, query string) {
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
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
