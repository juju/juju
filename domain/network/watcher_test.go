// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
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

func (s *watcherSuite) TestWatchControllerRemoteEndpointsFQDNChanges(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "fqdn_address")
	svc := service.NewWatchableService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, loggertesting.WrapCheckLog(c)),
		func(context.Context) (service.ProviderWithNetworking, error) { return nil, coreerrors.NotSupported },
		func(context.Context) (service.ProviderWithZones, error) { return nil, coreerrors.NotSupported },
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := svc.WatchControllerRemoteEndpoints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	nodeUUID := uuid.MustNewUUID().String()
	addressUUID := uuid.MustNewUUID().String()
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO fqdn_address (uuid, address, scope_id) VALUES (?, ?, 1)`,
				addressUUID, "controller-0.controller-service-endpoints.controller.svc.cluster.local")
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid) VALUES (?, ?)`,
				nodeUUID, addressUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})
	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchWithAdd(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "subnet")

	svc := service.NewWatchableService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, loggertesting.WrapCheckLog(c)),
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

	// Add a new subnet.
	subnet := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
	}
	createdSubnetID, err := svc.AddSubnet(c.Context(), subnet)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	watcherC.AssertChange(createdSubnetID.String())
}

func (s *watcherSuite) TestWatchWithDelete(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "subnet")

	svc := service.NewWatchableService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, loggertesting.WrapCheckLog(c)),
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

	// Add a new subnet.
	subnet := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
	}
	createdSubnetID, err := svc.AddSubnet(c.Context(), subnet)
	c.Assert(err, tc.ErrorIsNil)
	// Delete the subnet.
	err = svc.RemoveSubnet(c.Context(), createdSubnetID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	watcherC.AssertChange(createdSubnetID.String())
}
