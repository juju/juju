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

type cloudServiceImportSuite struct {
	linkLayerBaseSuite
}

func TestCloudServiceImportSuite(t *testing.T) {
	tc.Run(t, &cloudServiceImportSuite{})
}

// TestCreateCloudServices tests the happy path for cloud service creation and deletion.
// It verifies that multiple cloud services are correctly inserted into the database
// and then properly deleted.
func (s *cloudServiceImportSuite) TestCreateCloudServices(c *tc.C) {
	// Arrange: Set up application for the cloud services
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID1 := s.addApplicationWithName(c, charmUUID, spaceUUID, "super-app-1")
	appUUID2 := s.addApplicationWithName(c, charmUUID, spaceUUID, "super-app-2")

	args := []internal.ImportCloudService{
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
	err := s.state.CreateCloudServices(c.Context(), args)

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
		rows, err := tx.QueryContext(ctx, `SELECT uuid, net_node_uuid, application_uuid, provider_id FROM k8s_service`)
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

func (s *cloudServiceImportSuite) fetchNetNodeUUIDs(c *tc.C) []string {
	var nodes []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT uuid FROM net_node`)
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
