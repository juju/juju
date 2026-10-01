// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type controllerNetworkWatcherSuite struct{ changestreamtesting.ModelSuite }

func TestControllerNetworkWatcherSuite(t *testing.T) { tc.Run(t, &controllerNetworkWatcherSuite{}) }

func (s *controllerNetworkWatcherSuite) TestNetworkLifecycle(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.ModelUUID())
	log := loggertesting.WrapCheckLog(c)
	svc := service.NewWatchableService(
		state.NewState(changestream.NewTxnRunnerFactory(factory), log),
		nil, nil,
		domain.NewWatcherFactory(factory, log),
		log,
	)
	w, err := svc.WatchControllerNetwork(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
	// No unit or net-node snapshot existed when the watcher subscribed.
	s.exec(c, `INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type, is_controller_model)
VALUES ('model', 'controller', 'controller', 'admin', 'caas', 'test', 'test', true)`,
		`INSERT INTO net_node (uuid) VALUES ('old'), ('new')`,
		`INSERT INTO charm (uuid, reference_name, create_time) VALUES ('charm', 'controller', '2026-01-01')`,
		`INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES ('app', 'controller', 0, 'charm', '`+network.AlphaSpaceId.String()+`')`,
		`INSERT INTO application_controller (application_uuid) VALUES ('app')`,
		`INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES ('unit', 'controller/0', 0, 'app', 'charm', 'old')`,
		`INSERT INTO fqdn_address (uuid, address, scope_id) VALUES ('old-dns', 'old.example.com', 1)`,
		`INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid) VALUES ('old', 'old-dns')`)
	wc.AssertOneChange()
	addresses, err := svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.HasLen, 1)
	c.Check(addresses[0].Value, tc.Equals, "old.example.com")

	s.exec(c, `UPDATE unit SET net_node_uuid = 'new'`)
	wc.AssertOneChange()
	addresses, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.HasLen, 0)
	// An address arriving on the replacement node must still notify.
	s.exec(c, `INSERT INTO fqdn_address (uuid, address, scope_id) VALUES ('new-dns', 'new.example.com', 1)`,
		`INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid) VALUES ('new', 'new-dns')`)
	wc.AssertOneChange()
	addresses, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.HasLen, 1)
	c.Check(addresses[0].Value, tc.Equals, "new.example.com")

	for _, statement := range []string{
		`UPDATE fqdn_address SET address = 'changed.example.com' WHERE uuid = 'new-dns'`,
		`DELETE FROM net_node_fqdn_address WHERE net_node_uuid = 'new'`,
		`INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, device_type_id, virtual_port_type_id) VALUES ('device', 'new', 'eth0', 1500, 2, 0)`,
		`INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, config_type_id, origin_id, scope_id) VALUES ('ip', 'new', 'device', '10.0.0.2/24', 0, 4, 1, 3)`,
		`UPDATE link_layer_device SET device_type_id = 7 WHERE uuid = 'device'`,
		`INSERT INTO space (uuid, name) VALUES ('management', 'management')`,
		`INSERT INTO subnet (uuid, cidr, space_uuid) VALUES ('subnet', '10.0.0.0/24', 'management')`,
		`UPDATE ip_address SET subnet_uuid = 'subnet' WHERE uuid = 'ip'`,
		`UPDATE subnet SET space_uuid = '` + network.AlphaSpaceId.String() + `'`,
		`UPDATE space SET name = 'renamed' WHERE uuid = 'management'`,
		`UPDATE unit SET life_id = 1`,
		`UPDATE application SET life_id = 1`,
	} {
		s.exec(c, statement)
		wc.AssertOneChange()
	}
	addresses, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.HasLen, 1)
	c.Check(addresses[0].Value, tc.Equals, "10.0.0.2")
	c.Check(addresses[0].SpaceID, tc.Equals, network.AlphaSpaceId)
	s.exec(c, `UPDATE unit SET life_id = 2`)
	wc.AssertOneChange()
	_, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *controllerNetworkWatcherSuite) exec(c *tc.C, statements ...string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}
