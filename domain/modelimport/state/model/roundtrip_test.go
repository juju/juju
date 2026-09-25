// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/v4_2_0"
	importstate "github.com/juju/juju/domain/modelimport/state/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type roundTripSuite struct {
	schematesting.ModelSuite
}

func TestRoundTripSuite(t *testing.T) {
	tc.Run(t, &roundTripSuite{})
}

// TestImportExportRoundTrip imports a payload and re-exports it, asserting the
// generated importer is a faithful write-mirror of the exporter. The payload
// deliberately covers three cases:
//   - sequence: an FK-free content table (exact round-trip).
//   - provider_space -> space: a foreign key whose child table ("provider_space")
//     sorts before its parent ("space"), so the child is inserted before the
//     parent. This only succeeds because the importer defers foreign-key checks
//     to commit; with eager checks (which the model DB enforces by default) it
//     would fail.
//   - space: a seeded-extensible table. Its well-known alpha seed row is skipped
//     by ON CONFLICT DO NOTHING (not duplicated) while the user-created space is
//     inserted.
func (s *roundTripSuite) TestImportExportRoundTrip(c *tc.C) {
	const userSpaceUUID = "11111111-1111-1111-1111-111111111111"
	s.bootstrapModel(c)

	payload := &v4_2_0.ModelExport{
		Sequence: []v4_2_0.Sequence{
			{Namespace: "machine", Value: 7},
			{Namespace: "unit", Value: 3},
		},
		Space: []v4_2_0.Space{
			{UUID: userSpaceUUID, Name: "user-space"},
		},
		ProviderSpace: []v4_2_0.ProviderSpace{
			{ProviderID: "provider-1", SpaceUUID: userSpaceUUID},
		},
	}

	importSt := importstate.NewState(s.TxnRunnerFactory())
	err := importSt.Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)

	exportSt := exportstate.NewState(s.TxnRunnerFactory())
	got, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// FK-free content tables round-trip exactly.
	c.Check(got.Sequence, tc.SameContents, payload.Sequence)
	c.Check(got.ProviderSpace, tc.SameContents, payload.ProviderSpace)

	// The user space was imported; the alpha seed row was preserved exactly once
	// (skipped by ON CONFLICT DO NOTHING, not duplicated).
	var userSpaces, alphaSpaces int
	for _, sp := range got.Space {
		switch sp.UUID {
		case userSpaceUUID:
			c.Check(sp.Name, tc.Equals, "user-space")
			userSpaces++
		default:
			if sp.Name == "alpha" {
				alphaSpaces++
			}
		}
	}
	c.Check(userSpaces, tc.Equals, 1)
	c.Check(alphaSpaces, tc.Equals, 1)
}

// TestDeviceLessAddressRoundTrip covers the Kubernetes case: pod and service
// addresses carry no link-layer device (device_uuid is NULL). The payload must
// round-trip without the exporter collapsing NULL to an empty string, which
// would fail the ip_address device foreign key on import.
func (s *roundTripSuite) TestDeviceLessAddressRoundTrip(c *tc.C) {
	const (
		nodeUUID = "22222222-2222-2222-2222-222222222222"
		addrUUID = "33333333-3333-3333-3333-333333333333"
	)
	s.bootstrapModel(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value,
    type_id, config_type_id, origin_id, scope_id)
VALUES (?, ?, NULL, "10.0.0.1/24", 0, 4, 1, 2)
`, addrUUID, nodeUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	exportSt := exportstate.NewState(s.TxnRunnerFactory())
	exported, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exported.IpAddress, tc.HasLen, 1)
	c.Check(exported.IpAddress[0].DeviceUUID, tc.IsNil)

	// Wipe and re-import the exported payload.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM ip_address`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM net_node`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	importSt := importstate.NewState(s.TxnRunnerFactory())
	err = importSt.Import(c.Context(), exported)
	c.Assert(err, tc.ErrorIsNil)

	reexported, err := exportSt.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reexported.IpAddress, tc.DeepEquals, exported.IpAddress)
}

func (s *roundTripSuite) bootstrapModel(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test-model", "test-qualifier", "iaas", "test-cloud", "test-cloud-type")
`, s.ModelUUID(), "controller-uuid"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO model_agent (model_uuid) VALUES (?)`, s.ModelUUID())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
