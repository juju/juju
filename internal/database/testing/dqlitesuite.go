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
	"github.com/juju/errors"
	"github.com/juju/tc"
	_ "github.com/mattn/go-sqlite3"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/client"
	"github.com/juju/juju/internal/database/dqlite"
	"github.com/juju/juju/internal/database/pragma"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

// includeSQLOutput is used to enable the output of all SQL queries hitting the
// database.
var includeSQLOutput = os.Getenv("INCLUDE_SQL_OUTPUT")

// SchemaApplier is an interface that can be used to apply a schema to a
// database.
type SchemaApplier interface {
	Apply(c *tc.C, ctx context.Context, runner coredatabase.TxnRunner)
}

// DqliteSuite is used to provide a sql.DB reference to tests.
// It is not pre-populated with any schema and is the job the users of this
// Suite to call ApplyDDL after SetupTest has been called.
type DqliteSuite struct {
	testhelpers.IsolationSuite

	// Verbose indicates whether the suite should print all the sql
	// hitting the db.
	Verbose bool

	// UseTCP when true, SetUpTest will use a random TCP port.
	// When false, it will use a random UNIX abstract domain socket.
	UseTCP bool

	dbPath   string
	rootPath string
	uniqueID int64

	dqlite    *app.App
	db        *sql.DB
	trackedDB coredatabase.TxnRunner
}

// SetUpTest creates a new sql.DB reference and ensures that the
// controller schema is applied successfully.
func (s *DqliteSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.rootPath = c.MkDir()

	path := filepath.Join(s.rootPath, "dqlite")
	err := os.Mkdir(path, 0700)
	c.Assert(err, tc.ErrorIsNil)
	s.dbPath = path
	c.Cleanup(func() {
		if dqlite.Enabled {
			return
		}
		// Sometimes an Sqlite3 database will be writing after the test has
		// finished, despite the database being closed properly. This brute
		// force directory removal ensures the testing framework does not
		// fail when removing the test temp directory.
		for err = error(nil); err == nil; _, err = os.Stat(s.dbPath) {
			_ = os.RemoveAll(s.dbPath)
		}
		s.dbPath = ""
	})

	endpoint := ""

	if s.UseTCP {
		port := FindTCPPort(c)
		endpoint = fmt.Sprintf("%s:%d", "127.0.0.1", port)
		c.Logf("Opening dqlite db with: %v", endpoint)
	} else {
		endpoint = "@" + uuid.MustNewUUID().String()
		c.Logf("Opening dqlite db on abstract domain socket: %q", endpoint)
	}

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
		app.WithAddress(endpoint),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() {
		err := s.dqlite.Close()
		c.Check(err, tc.ErrorIsNil)
		s.dqlite = nil
	})

	// Enable super verbose mode.
	s.Verbose = verbose && includeSQLOutput != ""

	err = s.dqlite.Ready(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	s.trackedDB, s.db = s.OpenDB(c)
}

// DB returns a sql.DB reference.
func (s *DqliteSuite) DB() *sql.DB {
	return s.db
}

// TxnRunner returns the suite's transaction runner.
func (s *DqliteSuite) TxnRunner() coredatabase.TxnRunner {
	return s.trackedDB
}

// DBApp returns the dqlite app.
func (s *DqliteSuite) DBApp() *app.App {
	return s.dqlite
}

// RootPath returns the root path for the dqlite database.
func (s *DqliteSuite) RootPath() string {
	return s.rootPath
}

// DBPath returns the path to the dqlite database.
func (s *DqliteSuite) DBPath() string {
	return s.dbPath
}

// ApplyDDL is a helper manager for the test suites to apply a set of DDL string
// on top of a pre-established database.
func (s *DqliteSuite) ApplyDDL(c *tc.C, schema SchemaApplier) {
	s.ApplyDDLForRunner(c, schema, s.trackedDB)
}

// ApplyDDLForRunner is a helper manager for the test suites to apply a set of
// DDL string on top of a pre-established database.
func (s *DqliteSuite) ApplyDDLForRunner(c *tc.C, schema SchemaApplier, runner coredatabase.TxnRunner) {
	schema.Apply(c, c.Context(), runner)
}

// OpenDB returns a new sql.DB reference.
func (s *DqliteSuite) OpenDB(c *tc.C) (coredatabase.TxnRunner, *sql.DB) {
	// Increment the id and use it as the database name, this prevents
	// tests from interfering with each other.
	uniqueID := atomic.AddInt64(&s.uniqueID, 1)
	return s.OpenDBForNamespace(c, strconv.FormatInt(uniqueID, 10), true)
}

// OpenDBForNamespace returns a new sql.DB reference for the domain.
func (s *DqliteSuite) OpenDBForNamespace(c *tc.C, domain string, foreignKey bool) (coredatabase.TxnRunner, *sql.DB) {
	// There are places in the Juju code where an empty model uuid is valid and
	// takes on a double meaning to signify something else. It's possible that
	// in test scenarios as we move to DQlite that these empty model uuid's can
	// flow down here. In that case the error message is very cryptic. So we
	// check for empty string here to go bang in a more understandable way.
	c.Assert(domain, tc.Not(tc.Equals), "", tc.Commentf("cannot open a database for a empty domain"))

	db, err := s.dqlite.Open(c.Context(), domain)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure we close all databases that are opened during the tests.
	c.Cleanup(func() {
		err := db.Close()
		c.Check(err, tc.ErrorIsNil)
	})

	err = pragma.SetPragma(c.Context(), db, pragma.ForeignKeysPragma, foreignKey)
	c.Assert(err, tc.ErrorIsNil)

	trackedDB := &txnRunner{
		db: sqlair.NewDB(db),
	}
	return trackedDB, trackedDB.db.PlainDB()
}

// TxnRunnerFactory returns a DBFactory that returns the given database.
func (s *DqliteSuite) TxnRunnerFactory() func() (coredatabase.TxnRunner, error) {
	return func() (coredatabase.TxnRunner, error) {
		return s.trackedDB, nil
	}
}

// NoopTxnRunner returns a no-op transaction runner.
// Each call to this function will return the same instance, which will return
// a not implemented error when used.
func (s *DqliteSuite) NoopTxnRunner() coredatabase.TxnRunner {
	return noopTxnRunner{}
}

// DumpTable dumps the contents of the given table to stdout.
// This is useful for debugging tests. It is not intended for use
// in production code.
func (s *DqliteSuite) DumpTable(c *tc.C, table string, additionalTables ...string) {
	DumpTable(c, s.DB(), table, additionalTables...)
}

// FindTCPPort finds an unused TCP port and returns it.
// It is prone to racing, so the port should be used as soon as it is acquired
// to minimise the change of another process using it in the interim.
// The chances of this should be negligible during testing.
func FindTCPPort(c *tc.C) int {
	l, err := net.Listen("tcp", ":0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(l.Close(), tc.ErrorIsNil)
	return l.Addr().(*net.TCPAddr).Port
}

type noopTxnRunner struct{}

// Txn manages the application of a SQLair transaction within which the
// input function is executed. See https://github.com/canonical/sqlair.
// The input context can be used by the caller to cancel this process.
func (noopTxnRunner) Txn(context.Context, func(context.Context, *sqlair.TX) error) error {
	return errors.NotImplemented
}

// StdTxn manages the application of a standard library transaction within
// which the input function is executed.
// The input context can be used by the caller to cancel this process.
func (noopTxnRunner) StdTxn(context.Context, func(context.Context, *sql.Tx) error) error {
	return errors.NotImplemented
}
