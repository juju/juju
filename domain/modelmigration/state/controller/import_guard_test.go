// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/uuid"
)

func (s *stateSuite) TestImportTxnRunnerFactory(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	factory := NewImportTxnRunnerFactory(
		s.TxnRunnerFactory(), s.modelUUID.String(), claimUUID,
	)
	runner, err := factory(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	called := false
	err = runner.Txn(c.Context(), func(context.Context, *sqlair.TX) error {
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Check(runner.Dying(), tc.NotNil)

	err = runner.StdTxn(c.Context(), func(context.Context, *sql.Tx) error {
		c.Fatalf("standard transaction callback must not be called")
		return nil
	})
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *stateSuite) TestImportTxnRunnerFactoryRejectsWrongAttempt(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	for _, test := range []struct {
		name      string
		modelUUID string
		claimUUID string
	}{
		{name: "wrong model", modelUUID: uuid.MustNewUUID().String(), claimUUID: claimUUID},
		{name: "wrong claim", modelUUID: s.modelUUID.String(), claimUUID: uuid.MustNewUUID().String()},
	} {
		c.Logf("case %q", test.name)
		factory := NewImportTxnRunnerFactory(
			s.TxnRunnerFactory(), test.modelUUID, test.claimUUID,
		)
		runner, err := factory(c.Context())
		c.Assert(err, tc.ErrorIsNil)

		called := false
		err = runner.Txn(c.Context(), func(context.Context, *sqlair.TX) error {
			called = true
			return nil
		})
		c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
		c.Check(called, tc.IsFalse)
	}
}

func (s *stateSuite) TestImportTxnRunnerFactoryRejectsAborting(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	factory := NewImportTxnRunnerFactory(
		s.TxnRunnerFactory(), s.modelUUID.String(), claimUUID,
	)
	runner, err := factory(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(c.Context(), `
UPDATE model_migration_import SET phase_type_id = 2 WHERE uuid = ?`, claimUUID)
	c.Assert(err, tc.ErrorIsNil)

	called := false
	err = runner.Txn(c.Context(), func(context.Context, *sqlair.TX) error {
		called = true
		return nil
	})
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotImporting)
	c.Check(called, tc.IsFalse)
}

func (s *stateSuite) TestImportTxnRunnerFactoryRacesAbort(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(c.Context(), `
CREATE TABLE import_guard_probe (value TEXT NOT NULL)`)
	c.Assert(err, tc.ErrorIsNil)

	type probe struct {
		Value string `db:"value"`
	}
	insertStmt, err := sqlair.Prepare(`
INSERT INTO import_guard_probe (value) VALUES ($probe.value)`, probe{})
	c.Assert(err, tc.ErrorIsNil)

	factory := NewImportTxnRunnerFactory(
		s.TxnRunnerFactory(), s.modelUUID.String(), claimUUID,
	)
	runner, err := factory(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	start := make(chan struct{})
	writeResult := make(chan error, 1)
	phaseResult := make(chan error, 1)
	go func() {
		<-start
		writeResult <- runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			return tx.Query(ctx, insertStmt, probe{Value: "committed"}).Run()
		})
	}()
	go func() {
		<-start
		_, err := s.DB().ExecContext(c.Context(), `
UPDATE model_migration_import SET phase_type_id = 2 WHERE uuid = ?`, claimUUID)
		phaseResult <- err
	}()
	close(start)

	writeErr := <-writeResult
	phaseErr := <-phaseResult
	c.Assert(phaseErr, tc.ErrorIsNil)

	var count int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM import_guard_probe").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	if writeErr == nil {
		c.Check(count, tc.Equals, 1)
	} else {
		c.Check(writeErr, tc.ErrorIs, modelmigrationerrors.ErrImportNotImporting)
		c.Check(count, tc.Equals, 0)
	}
}
