// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchWithAdd(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "subnet")

	svc := service.NewWatchableService(
		state.NewState(func() (database.TxnRunner, error) { return factory() }, loggertesting.WrapCheckLog(c)),
		nil, nil,
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := svc.WatchSubnets(context.Background(), set.NewStrings())
	c.Assert(err, jc.ErrorIsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	// Add a new subnet.
	subnet := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
	}
	createdSubnetID, err := svc.AddSubnet(context.Background(), subnet)
	c.Assert(err, jc.ErrorIsNil)

	// Get the change.
	watcherC.AssertChange(createdSubnetID.String())
}

func (s *watcherSuite) TestWatchWithDelete(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "subnet")

	svc := service.NewWatchableService(
		state.NewState(func() (database.TxnRunner, error) { return factory() }, loggertesting.WrapCheckLog(c)),
		nil, nil,
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := svc.WatchSubnets(context.Background(), set.NewStrings())
	c.Assert(err, jc.ErrorIsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	// Add a new subnet.
	subnet := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
	}
	createdSubnetID, err := svc.AddSubnet(context.Background(), subnet)
	c.Assert(err, jc.ErrorIsNil)
	// Delete the subnet.
	err = svc.RemoveSubnet(context.Background(), createdSubnetID.String())
	c.Assert(err, jc.ErrorIsNil)

	// Get the change.
	watcherC.AssertChange(createdSubnetID.String())
}
