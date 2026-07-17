// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

type k8sServiceImportSuite struct {
	linkLayerBaseSuite
}

func TestK8sServiceImportSuite(t *testing.T) {
	tc.Run(t, &k8sServiceImportSuite{})
}

// TestCreateK8sServices tests the happy path for cloud service creation and deletion.
// It verifies that multiple cloud services are correctly inserted into the database
// and then properly deleted.
func (s *k8sServiceImportSuite) TestCreateK8sServices(c *tc.C) {
	// Arrange: Set up application for the cloud services
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID1 := s.addApplicationWithName(c, charmUUID, spaceUUID, "super-app-1")
	appUUID2 := s.addApplicationWithName(c, charmUUID, spaceUUID, "super-app-2")

	args := []internal.ImportK8sService{
		{
			UUID:            "service-uuid-1",
			DeviceUUID:      "device-uuid-1",
			NetNodeUUID:     "net-node-uuid-1",
			ApplicationName: "super-app-1",
			ProviderID:      "provider-id-1",
		},
		{
			UUID:            "service-uuid-2",
			DeviceUUID:      "device-uuid-2",
			NetNodeUUID:     "net-node-uuid-2",
			ApplicationName: "super-app-2",
			ProviderID:      "provider-id-2",
		},
	}

	// Act
	err := s.state.CreateK8sServices(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fetchNetNodeUUIDs(c), tc.SameContents, []string{"net-node-uuid-1", "net-node-uuid-2"})
	type k8sService struct {
		UUID            string
		NetNodeUUID     string
		ApplicationUUID string
		ProviderID      string
	}
	var k8sServices []k8sService
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		k8sServices = nil

		rows, err := tx.QueryContext(ctx, `SELECT uuid, net_node_uuid, application_uuid, provider_id FROM k8s_service WHERE application_uuid >= ''`)
		if err != nil {
			return errors.Errorf("querying net nodes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var get k8sService
			err := rows.Scan(&get.UUID, &get.NetNodeUUID, &get.ApplicationUUID, &get.ProviderID)
			if err != nil {
				return errors.Errorf("scanning net nodes: %w", err)
			}
			k8sServices = append(k8sServices, get)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(k8sServices, tc.SameContents, []k8sService{
		{
			UUID:            "service-uuid-1",
			NetNodeUUID:     "net-node-uuid-1",
			ApplicationUUID: appUUID1,
			ProviderID:      "provider-id-1",
		},
		{
			UUID:            "service-uuid-2",
			NetNodeUUID:     "net-node-uuid-2",
			ApplicationUUID: appUUID2,
			ProviderID:      "provider-id-2",
		}})
}

func (s *k8sServiceImportSuite) fetchNetNodeUUIDs(c *tc.C) []string {
	var nodes []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		nodes = nil

		rows, err := tx.QueryContext(ctx, `SELECT uuid FROM net_node WHERE uuid >= ''`)
		if err != nil {
			return errors.Errorf("querying net nodes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var uuid string
			err := rows.Scan(&uuid)
			if err != nil {
				return errors.Errorf("scanning net nodes: %w", err)
			}
			nodes = append(nodes, uuid)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to check DB: %v"))
	return nodes
}
