// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_0_12"
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

func (s *importSuite) TestImportRejectsMissingModelAgent(c *tc.C) {
	s.bootstrapModel(c)

	err := modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), &latest.ModelExport{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportRejectsMultipleModelAgents(c *tc.C) {
	s.bootstrapModel(c)

	passwordHash := "hash"
	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{
			{ModelUUID: s.ModelUUID(), PasswordHash: &passwordHash},
			{ModelUUID: s.ModelUUID(), PasswordHash: &passwordHash},
		},
	}

	err := modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestImportSanitizesCharmBlobResidency asserts that a charm row carrying
// the source's blob-residency state (available/archive_path/object_store_uuid)
// lands on the target as not-yet-available. The source's object_store_uuid
// refers to a blob on the source controller, not this one; object_store_metadata
// itself is excluded from import (see nonContentTables in
// generate/modelimport/main.go), so an unsanitized reference would dangle
// and fail the deferred foreign-key check when state.Import's own
// transaction commits. Only the binary-transfer phase that follows Import
// proves the blob has actually arrived here.
func (s *importSuite) TestImportSanitizesCharmBlobResidency(c *tc.C) {
	s.bootstrapModel(c)

	charmUUID := "charm-uuid"
	archivePath := "/some/path"
	objectStoreUUID := "00000000-0000-0000-0000-000000000000"
	available := true
	architectureID := int64(0)
	passwordHash := "hash"

	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{{
			ModelUUID:    s.ModelUUID(),
			PasswordHash: &passwordHash,
		}},
		// The source also exports the object_store_metadata row backing the
		// charm's blob, but that table is excluded from import entirely: this
		// deliberately leaves ObjectStoreUUID below dangling, matching what a
		// real source payload looks like once the charm row is sanitized.
		ObjectStoreMetadata: []v4_1_0.ObjectStoreMetadata{{
			UUID:   objectStoreUUID,
			Sha256: "sha256",
			Sha384: "sha384",
			Size:   1,
		}},
		Charm: []v4_1_0.Charm{{
			UUID:            charmUUID,
			ArchivePath:     &archivePath,
			ObjectStoreUUID: &objectStoreUUID,
			Available:       &available,
			SourceID:        0,
			Revision:        1,
			ArchitectureID:  &architectureID,
			ReferenceName:   "ubuntu",
		}},
		// The source also exports the charm's verified hash, but charm_hash is
		// a binary-residency table excluded from import (see nonContentTables
		// in generate/modelimport/main.go): the importer must ignore this row,
		// leaving the binary-transfer phase to insert the real hash once the
		// archive actually lands here. Were it imported, that phase's insert
		// would collide (charm_hash is insert-only, see its "unmodifiable"
		// trigger).
		CharmHash: []v4_1_0.CharmHash{{
			CharmUUID: charmUUID,
			Hash:      "source-hash",
		}},
	}

	err := modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)

	var gotAvailable bool
	var gotArchivePath, gotObjectStoreUUID sql.NullString
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT available, archive_path, object_store_uuid
FROM   charm
WHERE  uuid = ?
`, charmUUID).Scan(&gotAvailable, &gotArchivePath, &gotObjectStoreUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotAvailable, tc.IsFalse)
	c.Check(gotArchivePath.Valid, tc.IsFalse)
	c.Check(gotObjectStoreUUID.Valid, tc.IsFalse)

	// charm_hash is excluded from import, so the source's hash never landed:
	// the row count stays zero until the binary-transfer phase inserts the
	// real one.
	var hashCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM charm_hash WHERE charm_uuid = ?`, charmUUID).Scan(&hashCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hashCount, tc.Equals, 0)
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

// TestImportTransformsLegacyTextIDs verifies that a 4.0.12 payload, whose
// identifier columns were TEXT, is transformed, imported and stored as
// INTEGER values in the model DB.
func (s *importSuite) TestImportTransformsLegacyTextIDs(c *tc.C) {
	const (
		charmUUID   = "22222222-2222-2222-2222-222222222222"
		appUUID     = "33333333-3333-3333-3333-333333333333"
		appNode     = "44444444-4444-4444-4444-444444444444"
		unitUUID    = "55555555-5555-5555-5555-555555555555"
		machineUUID = "66666666-6666-6666-6666-666666666666"
		machineNode = "77777777-7777-7777-7777-777777777777"
	)
	s.bootstrapModel(c)

	legacy := v4_0_12.ModelExport{
		Charm: []v4_0_12.Charm{{
			UUID: charmUUID, SourceID: 1, Revision: 1, ReferenceName: "test-charm",
			CreateTime: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		}},
		CharmConfig: []v4_0_12.CharmConfig{{
			CharmUUID: charmUUID, Key: "opt", TypeID: new("1"),
		}},
		NetNode: []v4_0_12.NetNode{{UUID: appNode}, {UUID: machineNode}},
		Application: []v4_0_12.Application{{
			UUID: appUUID, Name: "test-app", CharmUUID: charmUUID,
			SpaceUUID: "656b4a82-e28c-53d6-a014-f0dd53417eb6",
		}},
		ApplicationAgent: []v4_0_12.ApplicationAgent{{
			ApplicationUUID: appUUID, PasswordHashAlgorithmID: new("0"), PasswordHash: new("hash"),
		}},
		ApplicationPlatform: []v4_0_12.ApplicationPlatform{{
			ApplicationUUID: appUUID, OsID: "0", ArchitectureID: 0,
		}},
		Unit: []v4_0_12.Unit{{
			UUID: unitUUID, Name: "test-app/0", ApplicationUUID: appUUID,
			NetNodeUUID: appNode, CharmUUID: charmUUID,
			PasswordHashAlgorithmID: new("0"), PasswordHash: new("hash"),
		}},
		Machine: []v4_0_12.Machine{{
			UUID: machineUUID, Name: "0", NetNodeUUID: machineNode,
			PasswordHashAlgorithmID: new("0"), PasswordHash: new("hash"),
		}},
		MachinePlatform: []v4_0_12.MachinePlatform{{
			MachineUUID: machineUUID, OsID: "0", ArchitectureID: 0,
		}},
		ModelAgent: []v4_0_12.ModelAgent{{
			ModelUUID: s.ModelUUID(), PasswordHashAlgorithmID: new(int64(0)), PasswordHash: new("hash"),
		}},
	}

	tr, err := modelimport.NewTransformer()
	c.Assert(err, tc.ErrorIsNil)
	result, err := tr.Transform(c.Context(), semversion.MustParse("4.0.12"), legacy)
	c.Assert(err, tc.ErrorIsNil)
	transformed, ok := result.(latest.ModelExport)
	c.Assert(ok, tc.IsTrue)

	err = modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), &transformed)
	c.Assert(err, tc.ErrorIsNil)

	got, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.CharmConfig, tc.HasLen, 1)
	c.Check(got.CharmConfig[0].TypeID, tc.DeepEquals, new(int64(1)))
	c.Assert(got.Unit, tc.HasLen, 1)
	c.Check(got.Unit[0].PasswordHashAlgorithmID, tc.DeepEquals, new(int64(0)))
	c.Assert(got.Machine, tc.HasLen, 1)
	c.Check(got.Machine[0].PasswordHashAlgorithmID, tc.DeepEquals, new(int64(0)))
	c.Assert(got.ApplicationAgent, tc.HasLen, 1)
	c.Check(got.ApplicationAgent[0].PasswordHashAlgorithmID, tc.DeepEquals, new(int64(0)))
	c.Check(got.ApplicationPlatform[0].OsID, tc.Equals, int64(0))
	c.Check(got.MachinePlatform[0].OsID, tc.Equals, int64(0))

	// SQLite coerces by affinity, so also prove the stored values are
	// integers rather than text that merely scans as one.
	for _, q := range []string{
		`SELECT typeof(type_id) FROM charm_config`,
		`SELECT typeof(password_hash_algorithm_id) FROM unit`,
		`SELECT typeof(password_hash_algorithm_id) FROM machine`,
		`SELECT typeof(password_hash_algorithm_id) FROM application_agent`,
		`SELECT typeof(os_id) FROM application_platform`,
		`SELECT typeof(os_id) FROM machine_platform`,
	} {
		var typ string
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, q).Scan(&typ)
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(typ, tc.Equals, "integer", tc.Commentf("query: %s", q))
	}
}
