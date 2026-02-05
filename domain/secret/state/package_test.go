// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsecret "github.com/juju/juju/domain/secret"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type baseSuite struct {
	schematesting.ModelSuite
	state *State
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
	})
}

// txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *baseSuite) txn(c *tc.C, fn func(ctx context.Context, tx *sqlair.TX) error) error {
	return tc.Must1(c, s.state.DB, c.Context()).Txn(c.Context(), fn)
}

// queryRows returns rows as a slice of maps for the given query.
// This is intended to be used with SELECT statements for assertions.
func (s *baseSuite) queryRows(c *tc.C, query string, args ...interface{}) []map[string]interface{} {
	var results []map[string]interface{}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return err
		}

		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return err
			}

			row := make(map[string]interface{})
			for i, col := range cols {
				row[col] = values[i]
			}
			results = append(results, row)
		}
		return rows.Err()
	})
	c.Assert(err, tc.IsNil, tc.Commentf("querying rows with query %q", query))
	return results
}

func (s *baseSuite) getApplicationUUID(c *tc.C, appName string) (coreapplication.UUID, error) {
	var uuid coreapplication.UUID
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		uuid, err = s.state.getApplicationUUID(ctx, tx, appName)
		return err
	})
	return uuid, err
}

func (s *baseSuite) getUnitUUID(c *tc.C, unitName coreunit.Name) (coreunit.UUID, error) {
	var uuid coreunit.UUID
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		uuid, err = s.state.getUnitUUID(ctx, tx, unitName)
		return err
	})
	return uuid, err
}

func (s *baseSuite) checkUserSecretLabelExists(c *tc.C, label string) (bool, error) {
	var exists bool
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = s.state.checkUserSecretLabelExists(ctx, tx, &label, "")
		return err
	})
	return exists, err
}

func (s *baseSuite) checkApplicationSecretLabelExists(c *tc.C, appUUID coreapplication.UUID,
	label string) (bool, error) {
	var exists bool
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = s.state.checkApplicationSecretLabelExists(ctx, tx, appUUID, &label, "")
		return err
	})
	return exists, err
}

func (s *baseSuite) checkUnitSecretLabelExists(c *tc.C, unitUUID coreunit.UUID, label string) (bool, error) {
	var exists bool
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = s.state.checkUnitSecretLabelExists(ctx, tx, unitUUID, &label, "")
		return err
	})
	return exists, err
}

func (s *baseSuite) createUserSecret(c *tc.C, version int, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams) error {
	return s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.createUserSecret(ctx, tx, version, uri, secret)
	})
}

func (s *baseSuite) createCharmApplicationSecret(c *tc.C, version int, uri *coresecrets.URI, appName string, secret domainsecret.UpsertSecretParams) error {
	return s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		appUUID, err := s.state.getApplicationUUID(ctx, tx, appName)
		if err != nil {
			return err
		}
		return s.state.createCharmApplicationSecret(ctx, tx, version, uri, appUUID, secret)
	})
}

func (s *baseSuite) createCharmUnitSecret(c *tc.C, version int, uri *coresecrets.URI, unitName coreunit.Name,
	secret domainsecret.UpsertSecretParams) error {
	return s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID, err := s.state.getUnitUUID(ctx, tx, unitName)
		if err != nil {
			return err
		}
		return s.state.createCharmUnitSecret(ctx, tx, version, uri, unitUUID, secret)
	})
}
