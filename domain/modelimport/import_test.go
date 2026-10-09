// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	exportstate "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport"
	domainnetwork "github.com/juju/juju/domain/network"
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

func (s *importSuite) TestImportRestoresPendingUnitsAndSequence(c *tc.C) {
	s.bootstrapModel(c)

	scale := int64(3)
	scaling := false
	passwordHash := "hash"
	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{{
			ModelUUID: s.ModelUUID(), PasswordHash: &passwordHash,
		}},
		Charm: []v4_1_0.Charm{{
			UUID: "charm-uuid", SourceID: 0, Revision: 1,
			ReferenceName: "foo",
		}},
		Application: []v4_1_0.Application{{
			UUID: "application-uuid", Name: "foo", LifeID: 0,
			CharmUUID: "charm-uuid",
			SpaceUUID: "656b4a82-e28c-53d6-a014-f0dd53417eb6",
		}},
		ApplicationScale: []v4_1_0.ApplicationScale{{
			ApplicationUUID: "application-uuid", Scale: &scale,
			ScaleTarget: &scale, Scaling: &scaling, StartOrdinal: 2,
		}},
		NetNode: []v4_1_0.NetNode{{UUID: "imported-net-node"}},
		Unit: []v4_1_0.Unit{{
			UUID: "imported-unit", Name: "foo/2", LifeID: 0,
			ApplicationUUID: "application-uuid", NetNodeUUID: "imported-net-node",
			CharmUUID: "charm-uuid",
		}},
		Sequence: []v4_1_0.Sequence{{Namespace: "application_foo", Value: 4}},
	}

	err := modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)

	var names []string
	var highWater, start int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT name FROM unit ORDER BY name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			names = append(names, name)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT value FROM sequence WHERE namespace = 'application_foo'`).Scan(&highWater); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT start_ordinal FROM application_scale WHERE application_uuid = 'application-uuid'`).Scan(&start)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(names, tc.DeepEquals, []string{"foo/2", "foo/3", "foo/4"})
	c.Check(highWater, tc.Equals, 4)
	c.Check(start, tc.Equals, 2)

	// Reconciliation after a service restart must preserve the same identities.
	modelDB := func(context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	log := loggertesting.WrapCheckLog(c)
	restarted := applicationservice.NewMigrationService(
		applicationstate.NewState(modelDB, coremodel.UUID(s.ModelUUID()), clock.WallClock, log),
		clock.WallClock, log,
	)
	err = restarted.ReconcileImportedCAASUnits(c.Context(), "foo", true)
	c.Assert(err, tc.ErrorIsNil)
	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit WHERE application_uuid = 'application-uuid'`).Scan(&count); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT value FROM sequence WHERE namespace = 'application_foo'`).Scan(&highWater)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 3)
	c.Check(highWater, tc.Equals, 4)
}

func (s *importSuite) TestImportAtZeroRetainsNextOrdinal(c *tc.C) {
	s.bootstrapModel(c)

	zero := int64(0)
	scaling := false
	passwordHash := "hash"
	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{{
			ModelUUID: s.ModelUUID(), PasswordHash: &passwordHash,
		}},
		Charm: []v4_1_0.Charm{{
			UUID: "charm-uuid", SourceID: 0, Revision: 1,
			ReferenceName: "foo",
		}},
		Application: []v4_1_0.Application{{
			UUID: "application-uuid", Name: "foo", LifeID: 0,
			CharmUUID: "charm-uuid",
			SpaceUUID: "656b4a82-e28c-53d6-a014-f0dd53417eb6",
		}},
		ApplicationScale: []v4_1_0.ApplicationScale{{
			ApplicationUUID: "application-uuid", Scale: &zero,
			ScaleTarget: &zero, Scaling: &scaling,
		}},
		Sequence: []v4_1_0.Sequence{{Namespace: "application_foo", Value: 7}},
	}

	err := modelimport.NewImporter(s.TxnRunnerFactory()).Import(c.Context(), payload)
	c.Assert(err, tc.ErrorIsNil)

	var start int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT start_ordinal FROM application_scale WHERE application_uuid = 'application-uuid'`).Scan(&start)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(start, tc.Equals, 8)

	modelDB := func(context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	st := applicationstate.NewState(modelDB, coremodel.UUID(s.ModelUUID()), clock.WallClock, loggertesting.WrapCheckLog(c))
	names, err := st.AddCAASUnits(c.Context(), coreapplication.UUID("application-uuid"), application.AddCAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			UnitUUID:    tc.Must(c, coreunit.NewUUID),
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(names, tc.DeepEquals, []coreunit.Name{"foo/8"})
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
