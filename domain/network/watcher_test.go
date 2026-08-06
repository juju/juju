// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchWithAdd(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "subnet")

	st := state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, loggertesting.WrapCheckLog(c))
	svc := service.NewWatchableService(
		st,
		nil, nil,
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := svc.WatchSubnets(c.Context(), set.NewStrings())
	c.Assert(err, tc.ErrorIsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c, "before watcher start")

	// Add a new subnet via the migration service so the watcher test
	// exercises the service→state path for subnet creation.
	subnet := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
	}
	subnetUUID := uuid.MustNewUUID()
	err = service.NewMigrationService(st, loggertesting.WrapCheckLog(c)).ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              domainnetwork.SubnetUUID(subnetUUID.String()),
			CIDR:              subnet.CIDR,
			ProviderId:        subnet.ProviderId,
			ProviderNetworkId: subnet.ProviderNetworkId,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	watcherC.AssertChange(subnetUUID.String())
}

func (s *watcherSuite) TestWatchWithDelete(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "subnet")

	st := state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, loggertesting.WrapCheckLog(c))
	svc := service.NewWatchableService(
		st,
		nil, nil,
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := svc.WatchSubnets(c.Context(), set.NewStrings())
	c.Assert(err, tc.ErrorIsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c, "before watcher start")

	// Add a new subnet via the migration service so the watcher test
	// exercises the service→state path for subnet creation.
	subnet := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
	}
	subnetUUID := uuid.MustNewUUID()
	err = service.NewMigrationService(st, loggertesting.WrapCheckLog(c)).ImportSubnets(
		c.Context(),
		[]domainnetwork.ImportSubnetArgs{{
			UUID:              domainnetwork.SubnetUUID(subnetUUID.String()),
			CIDR:              subnet.CIDR,
			ProviderId:        subnet.ProviderId,
			ProviderNetworkId: subnet.ProviderNetworkId,
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	// Delete the subnet.
	err = svc.RemoveSubnet(c.Context(), subnetUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	watcherC.AssertChange(subnetUUID.String())
}
