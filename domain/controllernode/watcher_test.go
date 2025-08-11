// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllernode_test

import (
	"context"
	"testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
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
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that we get the controller node created event.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddDqliteNode(ctx, "0", uint64(1), "10.0.0.1")
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get the update controller node event.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddDqliteNode(ctx, "0", uint64(1), "10.0.0.2")
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get a new controller node.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddDqliteNode(ctx, "0", uint64(2), "10.0.0.3")
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestControllerAPIAddresses(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "controller_api_address")

	ctx := c.Context()
	svc := s.setupService(c, factory)
	watcher, err := svc.WatchControllerAPIAddresses(ctx)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.9.9.32",
				},
			},
			NetPort: 42,
		},
	}
	// Ensure that we get the controller api address created event.
	harness.AddTest(c, func(c *tc.C) {
		args := controllernode.SetAPIAddressArgs{
			APIAddresses: map[string]network.SpaceHostPorts{
				"0": addrs,
			},
		}
		svc.SetAPIAddresses(ctx, args)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get the controller api address added event.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddDqliteNode(ctx, "1", uint64(1), "10.0.0.1")
		c.Assert(err, jc.ErrorIsNil)
		args := controllernode.SetAPIAddressArgs{
			APIAddresses: map[string]network.SpaceHostPorts{
				"1": addrs,
			},
		}
		svc.SetAPIAddresses(ctx, args)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get the controller api address updated event.
	harness.AddTest(c, func(c *tc.C) {
		addrs[0].Value = "10.43.25.2"
		args := controllernode.SetAPIAddressArgs{
			APIAddresses: map[string]network.SpaceHostPorts{
				"0": addrs,
			},
		}
		svc.SetAPIAddresses(ctx, args)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Ensure that we get the removed controller api address event.
	harness.AddTest(c, func(c *tc.C) {
		args := controllernode.SetAPIAddressArgs{
			APIAddresses: map[string]network.SpaceHostPorts{
				"0": {},
			},
		}
		svc.SetAPIAddresses(ctx, args)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Nothing happens so no change.
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
