// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/client"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type bootstrapSuite struct {
	testhelpers.IsolationSuite
}

func TestBootstrapSuite(t *stdtesting.T) { tc.Run(t, &bootstrapSuite{}) }
func (s *bootstrapSuite) TestBootstrapSuccess(c *tc.C) {
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
			if bindAddress != "127.0.0.1" {
				return fmt.Errorf("expected dqlite_bind_address to be 127.0.0.1")
			}

			return nil
		})
	}

	err := BootstrapDqlite(c.Context(), mgr, modeltesting.GenModelUUID(c), loggertesting.WrapCheckLog(c), check)
	c.Assert(err, tc.ErrorIsNil)

}

type testNodeManager struct {
	c       *tc.C
	dataDir string
	port    int
}

func (f *testNodeManager) EnsureDataDir() (string, error) {
	if f.dataDir == "" {
		f.dataDir = f.c.MkDir()
	}
	return f.dataDir, nil
}

func (f *testNodeManager) IsLoopbackPreferred() bool {
	return true
}

func (f *testNodeManager) WithPreferredCloudLocalAddressOption(network.ConfigSource) (app.Option, error) {
	return f.WithLoopbackAddressOption(), nil
}

func (f *testNodeManager) WithLoopbackAddressOption() app.Option {
	if f.port == 0 {
		l, err := net.Listen("tcp", ":0")
		f.c.Assert(err, tc.ErrorIsNil)
		f.c.Assert(l.Close(), tc.ErrorIsNil)
		f.port = l.Addr().(*net.TCPAddr).Port
	}
	return app.WithAddress(fmt.Sprintf("127.0.0.1:%d", f.port))
}

func (f *testNodeManager) WithLogFuncOption() app.Option {
	return app.WithLogFunc(func(_ client.LogLevel, msg string, args ...interface{}) {
		f.c.Logf(msg, args...)
	})
}

func (f *testNodeManager) WithTracingOption() app.Option {
	return app.WithTracing(client.LogNone)
}

func (f *testNodeManager) WithTLSOption() (app.Option, error) {
	return nil, nil
}
