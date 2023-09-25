// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/client"
)

type bootstrapSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestBootstrapSuccess(c *gc.C) {
	mgr := &testNodeManager{c: c}

	// check tests the variadic operation functionality
	// and ensures that bootstrap applied the DDL.
	check := func(ctx context.Context, db database.TxnRunner) error {
		return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

			return nil
		})
	}

	err := BootstrapDqlite(context.Background(), mgr, stubLogger{}, true, check)
	c.Assert(err, jc.ErrorIsNil)
}

type testNodeManager struct {
	c       *gc.C
	dataDir string
	port    int
}

func (f *testNodeManager) EnsureDataDir() (string, error) {
	if f.dataDir == "" {
		f.dataDir = f.c.MkDir()
	}
	return f.dataDir, nil
}

func (f *testNodeManager) WithPreferredCloudLocalAddressOption(network.ConfigSource) (app.Option, error) {
	return f.WithLoopbackAddressOption(), nil
}

func (f *testNodeManager) WithLoopbackAddressOption() app.Option {
	if f.port == 0 {
		l, err := net.Listen("tcp", ":0")
		f.c.Assert(err, jc.ErrorIsNil)
		f.c.Assert(l.Close(), jc.ErrorIsNil)
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
