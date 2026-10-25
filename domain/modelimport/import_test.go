// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/export"
	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport"
	"github.com/juju/juju/domain/removal"
	removalstate "github.com/juju/juju/domain/removal/state/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

// TestImportLegacyRemovalForce takes a 4.0.12 payload, whose removal.force is
// an integer, through decode, transform, import and export, and checks that
// the flag is stored as a boolean and read back by the removal state.
func (s *importSuite) TestImportLegacyRemovalForce(c *tc.C) {
	s.bootstrapModel(c)
	scheduled := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	legacy := v4_0_12.ModelExport{
		ModelAgent: []v4_0_12.ModelAgent{{ModelUUID: s.ModelUUID()}},
		Removal: []v4_0_12.Removal{
			{UUID: "r-false", RemovalTypeID: 0, EntityUUID: "e1", Force: 0, ScheduledFor: scheduled},
			{UUID: "r-true", RemovalTypeID: 0, EntityUUID: "e2", Force: 1, ScheduledFor: scheduled, Arg: new("{}")},
		},
	}
	data, err := yaml.Marshal(legacy)
	c.Assert(err, tc.ErrorIsNil)

	payload, err := export.DecodePayload(semversion.MustParse("4.0.12"), data)
	c.Assert(err, tc.ErrorIsNil)
	transformer, err := modelimport.NewTransformer()
	c.Assert(err, tc.ErrorIsNil)
	transformed, err := transformer.Transform(c.Context(), semversion.MustParse("4.0.12"), payload)
	c.Assert(err, tc.ErrorIsNil)
	latestPayload, ok := transformed.(latest.ModelExport)
	c.Assert(ok, tc.IsTrue)

	err = modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), &latestPayload)
	c.Assert(err, tc.ErrorIsNil)

	exported, err := exportstate.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exported.Removal, tc.SameContents, []v4_1_0.Removal{
		{UUID: "r-false", RemovalTypeID: 0, EntityUUID: "e1", Force: false, ScheduledFor: scheduled},
		{UUID: "r-true", RemovalTypeID: 0, EntityUUID: "e2", Force: true, ScheduledFor: scheduled, Arg: new("{}")},
	})

	jobs, err := removalstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)).GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	forced := map[removal.UUID]bool{}
	for _, job := range jobs {
		forced[job.UUID] = job.Force
	}
	c.Check(forced, tc.DeepEquals, map[removal.UUID]bool{"r-false": false, "r-true": true})
}

// TestTransformLegacyRemovalForceInvalid verifies that a 4.0.12 payload with a
// removal.force other than 0 or 1 is refused before anything is imported.
func (s *importSuite) TestTransformLegacyRemovalForceInvalid(c *tc.C) {
	s.bootstrapModel(c)

	for _, force := range []int64{-1, 2} {
		data, err := yaml.Marshal(v4_0_12.ModelExport{
			ModelAgent: []v4_0_12.ModelAgent{{ModelUUID: s.ModelUUID()}},
			Removal:    []v4_0_12.Removal{{UUID: "r-bad", EntityUUID: "e1", Force: force}},
		})
		c.Assert(err, tc.ErrorIsNil)

		payload, err := export.DecodePayload(semversion.MustParse("4.0.12"), data)
		c.Assert(err, tc.ErrorIsNil)
		transformer, err := modelimport.NewTransformer()
		c.Assert(err, tc.ErrorIsNil)

		_, err = transformer.Transform(c.Context(), semversion.MustParse("4.0.12"), payload)
		c.Check(err, tc.ErrorMatches,
			fmt.Sprintf(`.*invalid removal force value %d for "r-bad": expected 0 or 1`, force))
	}

	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM removal`).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
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
