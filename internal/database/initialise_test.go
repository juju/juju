// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	coreschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/schema"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type initialisationSuite struct {
	testhelpers.IsolationSuite
}

func TestInitialisationSuite(t *testing.T) {
	tc.Run(t, &initialisationSuite{})
}

func (s *initialisationSuite) TestOpenDatabasesWithoutBootstrapData(c *tc.C) {
	const bindAddress = "10.0.0.1"
	addresses := network.NewMachineAddresses(
		[]string{bindAddress}, network.WithScope(network.ScopeCloudLocal),
	).AsProviderAddresses()
	mgr := &testNodeManager{c: c}
	var runners []coredatabase.TxnRunner
	err := WithDqlite(c.Context(), mgr, addresses, loggertesting.WrapCheckLog(c), func(ctx context.Context, session *DqliteSession) error {
		c.Check(session.NodeID(), tc.Not(tc.Equals), uint64(0))
		c.Check(session.BindAddress(), tc.Equals, bindAddress)

		controller, err := session.OpenDatabase(ctx, coredatabase.ControllerNS, schema.ControllerDDL())
		c.Assert(err, tc.ErrorIsNil)
		runners = append(runners, controller)
		var controllerNodeCount, foreignKeys int
		err = controller.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM controller_node").Scan(&controllerNodeCount); err != nil {
				return err
			}
			return tx.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys)
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(controllerNodeCount, tc.Equals, 0)
		c.Check(foreignKeys, tc.Equals, 1)

		// Schema setup can open multiple independent model namespaces.
		for range 2 {
			runner, err := session.OpenDatabase(ctx, tc.Must0(c, model.NewUUID).String(), schema.ModelDDL())
			c.Assert(err, tc.ErrorIsNil)
			runners = append(runners, runner)
			var count int
			err = runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
				if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM model").Scan(&count); err != nil {
					return err
				}
				// The same key can be inserted in each separate namespace.
				_, err := tx.ExecContext(ctx, `INSERT INTO model_config (key, value) VALUES ('name', 'controller')`)
				return err
			})
			c.Assert(err, tc.ErrorIsNil)
			c.Check(count, tc.Equals, 0)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	for _, runner := range runners {
		err := runner.StdTxn(c.Context(), func(context.Context, *sql.Tx) error {
			return errors.New("database remained usable after the session returned")
		})
		c.Check(err, tc.ErrorMatches, ".*database is closed.*")
	}
}

func (s *initialisationSuite) TestReadyFailureClosesApplication(c *tc.C) {
	expected := errors.New("readiness failed")
	application := &testInitialisationApp{c: c, readyErr: expected}
	err := withDqlite(c.Context(), application, "10.0.0.1", loggertesting.WrapCheckLog(c), func(context.Context, *DqliteSession) error {
		c.Fatal("initialisation ran before readiness")
		return nil
	})
	c.Check(err, tc.ErrorIs, expected)
	c.Check(application.closeCalls, tc.Equals, 1)
}

func (s *initialisationSuite) TestSuccessClosesSession(c *tc.C) {
	s.assertSessionClosed(c, nil, nil)
}

func (s *initialisationSuite) TestInitialisationFailureClosesSession(c *tc.C) {
	s.assertSessionClosed(c, errors.New("initialisation failed"), nil)
}

func (s *initialisationSuite) TestCloseFailurePreservesInitialisationError(c *tc.C) {
	s.assertSessionClosed(c, errors.New("initialisation failed"), errors.New("close failed"))
}

func (s *initialisationSuite) assertSessionClosed(c *tc.C, initialiseErr, closeErr error) {
	application := &testInitialisationApp{c: c, closeErr: closeErr}
	err := withDqlite(c.Context(), application, "10.0.0.1", loggertesting.WrapCheckLog(c), func(ctx context.Context, session *DqliteSession) error {
		_, err := session.OpenDatabase(ctx, coredatabase.ControllerNS, schema.ControllerDDL())
		c.Assert(err, tc.ErrorIsNil)
		return initialiseErr
	})
	if initialiseErr == nil {
		c.Check(err, tc.ErrorIsNil)
	} else {
		c.Check(err, tc.ErrorIs, initialiseErr)
	}
	c.Check(application.closeCalls, tc.Equals, 1)
}

func (s *initialisationSuite) TestSchemaFailureClosesSession(c *tc.C) {
	expected := errors.New("schema failed")
	application := &testInitialisationApp{c: c}
	err := withDqlite(c.Context(), application, "10.0.0.1", loggertesting.WrapCheckLog(c), func(ctx context.Context, session *DqliteSession) error {
		_, err := session.OpenDatabase(ctx, coredatabase.ControllerNS, failingInitialisationSchema{err: expected})
		return err
	})
	c.Check(err, tc.ErrorIs, expected)
	c.Check(application.closeCalls, tc.Equals, 1)
}

func (s *initialisationSuite) TestCancellationClosesSession(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	application := &testInitialisationApp{c: c}
	err := withDqlite(ctx, application, "10.0.0.1", loggertesting.WrapCheckLog(c), func(ctx context.Context, session *DqliteSession) error {
		_, err := session.OpenDatabase(ctx, coredatabase.ControllerNS, schema.ControllerDDL())
		c.Assert(err, tc.ErrorIsNil)
		cancel()
		return ctx.Err()
	})
	c.Check(err, tc.ErrorIs, context.Canceled)
	c.Check(application.closeCalls, tc.Equals, 1)
}

type testInitialisationApp struct {
	c          *tc.C
	readyErr   error
	closeErr   error
	closeCalls int
	databases  []*sql.DB
}

func (a *testInitialisationApp) Ready(context.Context) error {
	return a.readyErr
}

func (a *testInitialisationApp) Open(context.Context, string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err == nil {
		a.databases = append(a.databases, db)
		// Ensure a failing closure assertion does not leak the handle.
		a.c.Cleanup(func() { _ = db.Close() })
	}
	return db, err
}

func (a *testInitialisationApp) ID() uint64 {
	return 1
}

func (a *testInitialisationApp) Close() error {
	for _, db := range a.databases {
		a.c.Check(db.Ping(), tc.ErrorMatches, "sql: database is closed")
	}
	a.closeCalls++
	return a.closeErr
}

type failingInitialisationSchema struct {
	err error
}

func (s failingInitialisationSchema) Ensure(context.Context, coredatabase.TxnRunner) (coreschema.ChangeSet, error) {
	return coreschema.ChangeSet{}, s.err
}
