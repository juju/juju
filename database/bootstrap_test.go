// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"fmt"
	"net"

	"github.com/canonical/go-dqlite/app"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type bootstrapSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestBootstrapSuccess(c *gc.C) {
	opt := &testOptFactory{c: c}

	c.Assert(BootstrapDqlite(opt, stubLogger{}), jc.ErrorIsNil)

	// Now use the same options to reopen the controller database
	// and check that we can see bootstrap side effects.
	dir, err := opt.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)

	addrOpt, err := opt.WithAddressOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(dir, addrOpt)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { _ = dqlite.Close() })

	ctx := context.TODO()
	c.Assert(dqlite.Ready(ctx), jc.ErrorIsNil)

	db, err := dqlite.Open(ctx, "controller")
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { _ = db.Close() })

	rows, err := db.Query("SELECT COUNT(*) FROM lease_type")
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { _ = rows.Close() })

	var count int
	c.Assert(rows.Next(), jc.IsTrue)
	c.Assert(rows.Scan(&count), jc.ErrorIsNil)
	c.Check(count, gc.Equals, 3)
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
