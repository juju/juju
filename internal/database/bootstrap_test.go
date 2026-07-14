// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"testing"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/client"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type bootstrapSuite struct {
	testhelpers.IsolationSuite
}

func TestBootstrapSuite(t *testing.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestBootstrapSuccess(c *tc.C) {
	addresses := network.NewMachineAddresses(
		[]string{"127.0.0.1"}, network.WithScope(network.ScopeCloudLocal),
	).AsProviderAddresses()
	mgr := &testNodeManager{c: c, expectedAddresses: addresses}

	// check tests the variadic operation functionality
	// and ensures that bootstrap applied the DDL.
	check := func(ctx context.Context, controller, model database.TxnRunner) error {
		return controller.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, "SELECT COUNT(*) FROM lease_type")
			if err != nil {
				return err
			}
			defer func() { _ = rows.Close() }()

			if !rows.Next() {
				return errors.New("no rows in lease_type")
			}

			var count int
			err = rows.Scan(&count)
			if err != nil {
				return err
			}

			if count != 2 {
				return fmt.Errorf("expected 2 rows, got %d", count)
			}

			// Ensure we have a nodeID in the controller node.
			row := tx.QueryRowContext(ctx, "SELECT controller_id, dqlite_node_id, dqlite_bind_address FROM controller_node")
			var controllerID, nodeID uint64
			var bindAddress string
			err = row.Scan(&controllerID, &nodeID, &bindAddress)
			if err != nil {
				return err
			}

			if controllerID != 0 {
				return fmt.Errorf("expected controller_id to be 0, got %d", controllerID)
			}
			if nodeID == 0 {
				return fmt.Errorf("expected dqlite_node_id to be non-zero")
			}
			if bindAddress != "127.0.0.1" {
				return fmt.Errorf("expected dqlite_bind_address to be 127.0.0.1")
			}

			return nil
		})
	}

	err := BootstrapDqlite(
		c.Context(), mgr, addresses, tc.Must0(c, coremodel.NewUUID),
		loggertesting.WrapCheckLog(c), check,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mgr.addressOption, tc.Equals, "127.0.0.1")
	c.Check(mgr.tlsOptionCalled, tc.IsTrue)
}

func (s *bootstrapSuite) TestInsertControllerNodeIDPersistsHostname(c *tc.C) {
	db, err := sql.Open("sqlite3", ":memory:")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		c.Assert(db.Close(), tc.ErrorIsNil)
	}()

	_, err = db.Exec(`CREATE TABLE controller_node (
		controller_id TEXT PRIMARY KEY,
		dqlite_node_id INT NOT NULL,
		dqlite_bind_address TEXT NOT NULL
	)`)
	c.Assert(err, tc.ErrorIsNil)

	runner := &txnRunner{db: db}
	err = InsertControllerNodeID(
		c.Context(), runner, 42,
		"controller-0.controller-service-endpoints.test.svc.cluster.local",
	)
	c.Assert(err, tc.ErrorIsNil)

	var address string
	err = db.QueryRow(
		"SELECT dqlite_bind_address FROM controller_node WHERE controller_id = '0'",
	).Scan(&address)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		address, tc.Equals,
		"controller-0.controller-service-endpoints.test.svc.cluster.local",
	)
}

func (s *bootstrapSuite) TestInsertControllerNodeIDAlreadyExists(c *tc.C) {
	db, err := sql.Open("sqlite3", ":memory:")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		c.Assert(db.Close(), tc.ErrorIsNil)
	}()

	_, err = db.Exec(`CREATE TABLE controller_node (
		controller_id TEXT PRIMARY KEY,
		dqlite_node_id INT NOT NULL,
		dqlite_bind_address TEXT NOT NULL
	)`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`
		INSERT INTO controller_node (
			controller_id, dqlite_node_id, dqlite_bind_address
		) VALUES ('0', 42, 'controller-0.test')
	`)
	c.Assert(err, tc.ErrorIsNil)

	runner := &txnRunner{db: db}
	err = InsertControllerNodeID(c.Context(), runner, 43, "replacement.test")
	c.Assert(err, tc.ErrorIs, jujuerrors.AlreadyExists)

	var nodeID int
	var address string
	err = db.QueryRow(`
		SELECT dqlite_node_id, dqlite_bind_address
		FROM controller_node
		WHERE controller_id = '0'
	`).Scan(&nodeID, &address)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nodeID, tc.Equals, 42)
	c.Check(address, tc.Equals, "controller-0.test")
}

type testNodeManager struct {
	c                 *tc.C
	dataDir           string
	port              int
	expectedAddresses network.ProviderAddresses
	addressOption     string
	tlsOptionCalled   bool
}

func (f *testNodeManager) EnsureDataDir() (string, error) {
	if f.dataDir == "" {
		f.dataDir = f.c.MkDir()
	}
	return f.dataDir, nil
}

func (f *testNodeManager) ResolveBootstrapAddress(
	addresses network.ProviderAddresses, source network.ConfigSource,
) (string, error) {
	f.c.Check(addresses, tc.DeepEquals, f.expectedAddresses)
	f.c.Check(source, tc.NotNil)
	return "127.0.0.1", nil
}

func (f *testNodeManager) WithAddressOption(address string) app.Option {
	f.addressOption = address
	if f.port == 0 {
		l, err := net.Listen("tcp", ":0")
		f.c.Assert(err, tc.ErrorIsNil)
		f.c.Assert(l.Close(), tc.ErrorIsNil)
		f.port = l.Addr().(*net.TCPAddr).Port
	}
	return app.WithAddress(fmt.Sprintf("%s:%d", address, f.port))
}

func (f *testNodeManager) WithLogFuncOption() app.Option {
	return app.WithLogFunc(func(_ client.LogLevel, msg string, args ...any) {
		f.c.Logf(msg, args...)
	})
}

func (f *testNodeManager) WithTracingOption() app.Option {
	return app.WithTracing(client.LogNone)
}

func (f *testNodeManager) WithTLSOption() (app.Option, error) {
	f.tlsOptionCalled = true
	listen, dial, err := dqliteTLSConfig(
		jujutesting.CACert, jujutesting.ServerCert, jujutesting.ServerKey,
	)
	if err != nil {
		return nil, err
	}
	return app.WithTLS(listen, dial), nil
}
