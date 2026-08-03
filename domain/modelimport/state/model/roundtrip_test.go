// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/v4_1_0"
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

	payload := &v4_1_0.ModelExport{
		Sequence: []v4_1_0.Sequence{
			{Namespace: "machine", Value: 7},
			{Namespace: "unit", Value: 3},
		},
		Space: []v4_1_0.Space{
			{UUID: userSpaceUUID, Name: "user-space"},
		},
		ProviderSpace: []v4_1_0.ProviderSpace{
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
