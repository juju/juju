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

	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/client"
)

type bootstrapSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestBootstrapSuccess(c *gc.C) {
	opt := &testOptFactory{c: c}

	// check tests the variadic operation functionality
	// and ensures that bootstrap applied the DDL.
	check := func(db *sql.DB) error {
		rows, err := db.Query("SELECT COUNT(*) FROM lease_type")
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
	}

	err := BootstrapDqlite(context.TODO(), opt, stubLogger{}, check)
	c.Assert(err, jc.ErrorIsNil)
}

type testOptFactory struct {
	c       *gc.C
	dataDir string
	port    int
}

func (f *testOptFactory) EnsureDataDir() (string, error) {
	if f.dataDir == "" {
		f.dataDir = f.c.MkDir()
	}
	return f.dataDir, nil
}

func (f *testOptFactory) WithAddressOption() (app.Option, error) {
	if f.port == 0 {
		l, err := net.Listen("tcp", ":0")
		f.c.Assert(err, jc.ErrorIsNil)
		f.c.Assert(l.Close(), jc.ErrorIsNil)
		f.port = l.Addr().(*net.TCPAddr).Port
	}
	return app.WithAddress(fmt.Sprintf("127.0.0.1:%d", f.port)), nil
}

func (f *testOptFactory) WithLogFuncOption() app.Option {
	return app.WithLogFunc(func(_ client.LogLevel, msg string, args ...interface{}) {
		f.c.Logf(msg, args...)
	})
}
