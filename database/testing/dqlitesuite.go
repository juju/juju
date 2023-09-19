// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/canonical/sqlair"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/client"
	"github.com/juju/juju/database/pragma"
)

// SchemaApplier is an interface that can be used to apply a schema to a
// database.
type SchemaApplier interface {
	Apply(c *gc.C, ctx context.Context, runner coredatabase.TxnRunner)
}

// DqliteSuite is used to provide a sql.DB reference to tests.
// It is not pre-populated with any schema and is the job the users of this
// Suite to call ApplyDDL after SetupTest has been called.
type DqliteSuite struct {
	testing.IsolationSuite

	dbPath   string
	rootPath string
	uniqueID int64

	dqlite    *app.App
	db        *sql.DB
	trackedDB coredatabase.TxnRunner
}

// SetUpTest creates a new sql.DB reference and ensures that the
// controller schema is applied successfully.
func (s *DqliteSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.rootPath = c.MkDir()

	path := filepath.Join(s.rootPath, "dqlite")
	err := os.Mkdir(path, 0700)
	c.Assert(err, jc.ErrorIsNil)
	s.dbPath = path

	port := FindTCPPort(c)

	url := fmt.Sprintf("%s:%d", "[::1]", port)
	c.Logf("Opening dqlite db with: %v", url)

	// Depending on the verbosity of the test suite, we want to
	// also print all the sql hitting the db.
	var verbose bool
	flag.VisitAll(func(f *flag.Flag) {
		if verbose || !strings.Contains(f.Name, "check.vv") {
			return
		}
		verbose, _ = strconv.ParseBool(f.Value.String())
	})

	s.dqlite, err = app.New(s.dbPath,
		app.WithAddress(url),
		app.WithTracing(client.LogDebug),
		app.WithLogFunc(func(level client.LogLevel, msg string, args ...any) {
			switch level {
			case client.LogDebug:
				if !verbose {
					return
				}
				fallthrough
			case client.LogInfo, client.LogWarn, client.LogError:
				c.Logf("%s: %s, %v", level, msg, args)
			}
		}),
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.dqlite.Ready(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	s.trackedDB, s.db = s.OpenDB(c)
}

// TearDownTest is responsible for cleaning up the testing resources created
// with the ControllerSuite
func (s *DqliteSuite) TearDownTest(c *gc.C) {
	if s.dqlite != nil {
		err := s.dqlite.Close()
		c.Assert(err, jc.ErrorIsNil)
	}

	s.IsolationSuite.TearDownTest(c)
}

// DB returns a sql.DB reference.
func (s *DqliteSuite) DB() *sql.DB {
	return s.db
}

// TxnRunner returns the suite's transaction runner.
func (s *DqliteSuite) TxnRunner() coredatabase.TxnRunner {
	return s.trackedDB
}

func (s *DqliteSuite) DBApp() *app.App {
	return s.dqlite
}

func (s *DqliteSuite) RootPath() string {
	return s.rootPath
}

func (s *DqliteSuite) DBPath() string {
	return s.dbPath
}

// ApplyDDL is a helper manager for the test suites to apply a set of DDL string
// on top of a pre-established database.
func (s *DqliteSuite) ApplyDDL(c *gc.C, schema SchemaApplier) {
	s.ApplyDDLForRunner(c, schema, s.trackedDB)
}

// ApplyDDLForRunner is a helper manager for the test suites to apply a set of
// DDL string on top of a pre-established database.
func (s *DqliteSuite) ApplyDDLForRunner(c *gc.C, schema SchemaApplier, runner coredatabase.TxnRunner) {
	schema.Apply(c, context.Background(), runner)
}

// OpenDB returns a new sql.DB reference.
func (s *DqliteSuite) OpenDB(c *gc.C) (coredatabase.TxnRunner, *sql.DB) {
	// Increment the id and use it as the database name, this prevents
	// tests from interfering with each other.
	uniqueID := atomic.AddInt64(&s.uniqueID, 1)

	var err error
	s.db, err = s.dqlite.Open(context.Background(), strconv.FormatInt(uniqueID, 10))
	c.Assert(err, jc.ErrorIsNil)

	err = pragma.SetPragma(context.Background(), s.db, pragma.ForeignKeysPragma, true)
	c.Assert(err, jc.ErrorIsNil)

	trackedDB := &txnRunner{
		db: sqlair.NewDB(s.db),
	}

	return trackedDB, trackedDB.db.PlainDB()
}

// TxnRunnerFactory returns a DBFactory that returns the given database.
func (s *DqliteSuite) TxnRunnerFactory() func() (coredatabase.TxnRunner, error) {
	return func() (coredatabase.TxnRunner, error) {
		return s.trackedDB, nil
	}
}

// FindTCPPort finds an unused TCP port and returns it.
// It is prone to racing, so the port should be used as soon as it is acquired
// to minimise the change of another process using it in the interim.
// The chances of this should be negligible during testing.
func FindTCPPort(c *gc.C) int {
	l, err := net.Listen("tcp", "[::1]:0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(l.Close(), jc.ErrorIsNil)
	return l.Addr().(*net.TCPAddr).Port
}
