// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"testing"

	"github.com/juju/tc"

	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	importmigration "github.com/juju/juju/domain/modelimport/modelmigration"
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
	payload := &latest.ModelExport{
		Sequence: []v4_1_0.Sequence{{Namespace: "machine", Value: 5}},
	}

	err := importmigration.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)

	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Sequence, tc.SameContents, payload.Sequence)
}

// TestImportNilPayload verifies that importing a nil payload is a no-op.
func (s *importSuite) TestImportNilPayload(c *tc.C) {
	err := importmigration.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
}
