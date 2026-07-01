// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/export/types/v4_1_0"
	importstate "github.com/juju/juju/domain/modelimport/state/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type postImportSuite struct {
	schematesting.ModelSuite
}

func TestPostImportSuite(t *testing.T) {
	tc.Run(t, &postImportSuite{})
}

func (s *postImportSuite) TestMergeModelAgentPassword(c *tc.C) {
	s.bootstrapModel(c)

	passwordHash := "hash"
	passwordHashAlgorithmID := int64(0)
	st := importstate.NewState(s.TxnRunnerFactory())
	err := st.MergeModelAgentPassword(c.Context(), v4_1_0.ModelAgent{
		ModelUUID:               s.ModelUUID(),
		PasswordHash:            &passwordHash,
		PasswordHashAlgorithmID: &passwordHashAlgorithmID,
	})
	c.Assert(err, tc.ErrorIsNil)

	gotHash, gotAlgorithmID := s.modelAgentPassword(c)
	c.Check(gotHash.String, tc.Equals, passwordHash)
	c.Check(gotAlgorithmID.Int64, tc.Equals, passwordHashAlgorithmID)
}

// TestMergeModelAgentPasswordAllowsEmptyPassword asserts that merging a nil
// password (the normal state for a non-CAAS model, which never
// authenticates via the model tag) is a valid no-op, not a rejection.
func (s *postImportSuite) TestMergeModelAgentPasswordAllowsEmptyPassword(c *tc.C) {
	s.bootstrapModel(c)

	st := importstate.NewState(s.TxnRunnerFactory())
	err := st.MergeModelAgentPassword(c.Context(), v4_1_0.ModelAgent{ModelUUID: s.ModelUUID()})
	c.Assert(err, tc.ErrorIsNil)

	gotHash, gotAlgorithmID := s.modelAgentPassword(c)
	c.Check(gotHash.Valid, tc.IsFalse)
	c.Check(gotAlgorithmID.Valid, tc.IsFalse)
}

func (s *postImportSuite) TestMergeModelAgentPasswordNoMatchingRow(c *tc.C) {
	st := importstate.NewState(s.TxnRunnerFactory())
	err := st.MergeModelAgentPassword(c.Context(), v4_1_0.ModelAgent{ModelUUID: "does-not-exist"})
	c.Assert(err, tc.ErrorMatches, ".*affected 0 rows, expected 1.*")
}

func (s *postImportSuite) bootstrapModel(c *tc.C) {
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

func (s *postImportSuite) modelAgentPassword(c *tc.C) (sql.NullString, sql.NullInt64) {
	var hash sql.NullString
	var algorithmID sql.NullInt64
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT password_hash, password_hash_algorithm_id
FROM   model_agent
WHERE  model_uuid = ?
`, s.ModelUUID()).Scan(&hash, &algorithmID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return hash, algorithmID
}
