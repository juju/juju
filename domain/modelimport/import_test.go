// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type importSuite struct {
	schematesting.ModelSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

// TestImporterPerformsModelDBImport verifies NewImporter wires the model-DB
// state and that Import lands the payload in the model DB, read back via the
// export state (the import's read-mirror).
func (s *importSuite) TestImporterPerformsModelDBImport(c *tc.C) {
	s.bootstrapModel(c)

	passwordHash := "hash"
	passwordHashAlgorithmID := int64(0)
	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{{
			ModelUUID:               s.ModelUUID(),
			PasswordHashAlgorithmID: &passwordHashAlgorithmID,
			PasswordHash:            &passwordHash,
		}},
		Sequence: []v4_1_0.Sequence{{Namespace: "machine", Value: 5}},
	}

	err := modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)

	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Sequence, tc.SameContents, payload.Sequence)
	c.Check(got.ModelAgent, tc.DeepEquals, payload.ModelAgent)
}

// TestImportNilPayload verifies that importing a nil payload is a no-op.
func (s *importSuite) TestImportNilPayload(c *tc.C) {
	err := modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) bootstrapModel(c *tc.C) {
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
