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
	const bootstrapAddress = "10.0.0.1"
	addresses := network.NewMachineAddresses(
		[]string{bootstrapAddress}, network.WithScope(network.ScopeCloudLocal),
	).AsProviderAddresses()
	mgr := &testNodeManager{c: c}

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
			if bindAddress != bootstrapAddress {
				return fmt.Errorf(
					"expected dqlite_bind_address to be %q", bootstrapAddress,
				)
			}

			return nil
		})
	}

	err := BootstrapDqlite(
		c.Context(), mgr, addresses, tc.Must0(c, coremodel.NewUUID),
		loggertesting.WrapCheckLog(c), check,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mgr.addressOption, tc.Equals, bootstrapAddress)
	c.Check(mgr.tlsOptionCalled, tc.IsTrue)
}

func (s *bootstrapSuite) TestBootstrapNoAddress(c *tc.C) {
	mgr := &testNodeManager{c: c}

	err := BootstrapDqlite(
		c.Context(), mgr, nil, tc.Must0(c, coremodel.NewUUID),
		loggertesting.WrapCheckLog(c),
	)
	c.Check(err, tc.ErrorMatches, "Dqlite bootstrap address not found")
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

type testNodeManager struct {
	c               *tc.C
	dataDir         string
	port            int
	addressOption   string
	tlsOptionCalled bool
}

func (f *testNodeManager) EnsureDataDir() (string, error) {
	if f.dataDir == "" {
		f.dataDir = f.c.MkDir()
	}
	return f.dataDir, nil
}

func (f *testNodeManager) WithAddressOption(address string) app.Option {
	f.addressOption = address
	if f.port == 0 {
		l, err := net.Listen("tcp", ":0")
		f.c.Assert(err, tc.ErrorIsNil)
		f.c.Assert(l.Close(), tc.ErrorIsNil)
		f.port = l.Addr().(*net.TCPAddr).Port
	}
	// Bind the test Dqlite app to loopback so the provider address under test
	// does not need to exist on the host running the test.
	return app.WithAddress(fmt.Sprintf("127.0.0.1:%d", f.port))
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
