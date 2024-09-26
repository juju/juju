// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor_test

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/dqlite"
	"github.com/juju/juju/internal/database/pragma"
	databasetesting "github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/dbaccessor"
)

// dqliteAppIntegrationSuite defines a base suite for running integration
// tests against the Dqlite database. It overrides the various methods to
// prevent the creation of a new database for each test.
type dqliteAppIntegrationSuite struct {
	databasetesting.DqliteSuite
}

func (s *dqliteAppIntegrationSuite) TearDownSuite(c *gc.C) {
	// We don't call s.DBSuite.TearDownSuite here because
	// we don't want to double close the dqlite app.
	s.IsolationSuite.TearDownSuite(c)
}

func (s *dqliteAppIntegrationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	if !dqlite.Enabled {
		c.Skip("This requires a dqlite server to be running")
	}
}

func (s *dqliteAppIntegrationSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
}

type integrationSuite struct {
	dqliteAppIntegrationSuite

	db        *sql.DB
	dbGetter  coredatabase.DBGetter
	dbDeleter coredatabase.DBDeleter
	worker    worker.Worker
}

var _ = gc.Suite(&integrationSuite{})

func (s *integrationSuite) SetUpSuite(c *gc.C) {
	// This suite needs Dqlite setup on a tcp port.
	s.UseTCP = true
	s.dqliteAppIntegrationSuite.SetUpSuite(c)
}

func (s *integrationSuite) SetUpTest(c *gc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.dqliteAppIntegrationSuite.SetUpTest(c)

	params := agent.AgentConfigParams{
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Jobs:              []model.MachineJob{model.JobHostUnits},
		Password:          "sekrit",
		CACert:            "ca cert",
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
		Model:             testing.ModelTag,
		Controller:        testing.ControllerTag,
	}
	params.Paths.DataDir = s.RootPath()
	params.Paths.LogDir = c.MkDir()
	agentConfig, err := agent.NewAgentConfig(params)
	c.Assert(err, jc.ErrorIsNil)

	logger := loggertesting.WrapCheckLog(c)
	nodeManager := database.NewNodeManager(agentConfig, false, logger, coredatabase.NoopSlowQueryLogger{})

	db, err := s.DBApp().Open(context.Background(), coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)

	err = pragma.SetPragma(context.Background(), db, pragma.ForeignKeysPragma, true)
	c.Assert(err, jc.ErrorIsNil)

	runner := &txnRunner{db: db}

	err = database.NewDBMigration(
		runner, logger, schema.ControllerDDL()).Apply(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = database.InsertControllerNodeID(context.Background(), runner, s.DBApp().ID())
	c.Assert(err, jc.ErrorIsNil)

	w, err := dbaccessor.NewWorker(dbaccessor.WorkerConfig{
		NewApp: func(string, ...app.Option) (dbaccessor.DBApp, error) {
			return dbaccessor.WrapApp(s.DBApp()), nil
		},
		NewDBWorker:             dbaccessor.NewTrackedDBWorker,
		NodeManager:             nodeManager,
		MetricsCollector:        dbaccessor.NewMetricsCollector(),
		Clock:                   clock.WallClock,
		Logger:                  logger,
		ControllerID:            agentConfig.Tag().Id(),
		ControllerConfigWatcher: controllerConfigWatcher{},
		ClusterConfig:           clusterConfig{},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.db = db
	s.dbGetter = w
	s.dbDeleter = w
	s.worker = w
}

func (s *integrationSuite) TearDownTest(c *gc.C) {
	if dqlite.Enabled && s.worker != nil {
		workertest.CleanKill(c, s.worker)
	}
	if s.db != nil {
		s.db.Close()
	}
	s.dqliteAppIntegrationSuite.TearDownTest(c)
	s.DqliteSuite.TearDownTest(c)
}

func (s *integrationSuite) TestWorkerSetsNodeIDAndAddress(c *gc.C) {
	db, err := s.dbGetter.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)

	var (
		nodeID uint64
		addr   string
	)
	err = db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT dqlite_node_id, bind_address FROM controller_node WHERE controller_id = '0'")
		if err := row.Scan(&nodeID, &addr); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(nodeID, gc.Not(gc.Equals), uint64(0))
	c.Check(addr, gc.Equals, "127.0.0.1")
}

func (s *integrationSuite) TestWorkerAccessingControllerDB(c *gc.C) {
	db, err := s.dbGetter.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)
}

func (s *integrationSuite) TestWorkerAccessingUnknownDB(c *gc.C) {
	_, err := s.dbGetter.GetDB("foo")
	c.Assert(err, gc.ErrorMatches, `.*"foo": database not found`)
	c.Assert(err, jc.ErrorIs, coredatabase.ErrDBNotFound)
}

func (s *integrationSuite) TestWorkerAccessingKnownDB(c *gc.C) {
	db, err := s.dbGetter.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)

	err = db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO namespace_list (namespace) VALUES ("bar")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	db, err = s.dbGetter.GetDB("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Check that the model schema DDL was applied.
	type EditType struct {
		EditType string `db:"edit_type"`
	}
	var results []EditType
	q := sqlair.MustPrepare("SELECT &EditType.* FROM change_log_edit_type", EditType{})
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, q).GetAll(&results)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 3)
}

func (s *integrationSuite) TestWorkerDeletingControllerDB(c *gc.C) {
	err := s.dbDeleter.DeleteDB(coredatabase.ControllerNS)
	c.Assert(err, gc.ErrorMatches, `.*cannot delete controller database`)
}

func (s *integrationSuite) TestWorkerDeletingUnknownDB(c *gc.C) {
	err := s.dbDeleter.DeleteDB("foo")
	c.Assert(err, gc.ErrorMatches, `.*"foo" not found`)
}

func (s *integrationSuite) TestWorkerDeletingKnownDB(c *gc.C) {
	ctrlDB, err := s.dbGetter.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
	err = ctrlDB.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO namespace_list (namespace) VALUES ("baz")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	db, err := s.dbGetter.GetDB("baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// We need to unsure that we remove the namespace from the model list.
	// Otherwise, the db will be recreated on the next call to GetDB.
	err = ctrlDB.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM namespace_list WHERE namespace = "baz"`)
		return errors.Cause(err)
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.dbDeleter.DeleteDB("baz")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.dbGetter.GetDB("baz")
	c.Assert(err, gc.ErrorMatches, `.*namespace "baz": database not found`)
	c.Assert(err, jc.ErrorIs, coredatabase.ErrDBNotFound)
}

func (s *integrationSuite) TestWorkerDeleteKnownDBKillErr(c *gc.C) {
	ctrlDB, err := s.dbGetter.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
	err = ctrlDB.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO namespace_list (namespace) VALUES ("baz")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// GetDB ensures that we've got it cached.
	_, err = s.dbGetter.GetDB("baz")
	c.Assert(err, jc.ErrorIsNil)

	s.worker.Kill()
	err = s.dbDeleter.DeleteDB("baz")
	c.Assert(err, jc.ErrorIs, coredatabase.ErrDBAccessorDying)
}

// The following ensures that we can delete a db without having to call GetDB
// first. This ensures that we don't have to have an explicit db worker for
// each model.
func (s *integrationSuite) TestWorkerDeletingKnownDBWithoutGetFirst(c *gc.C) {
	ctrlDB, err := s.dbGetter.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
	err = ctrlDB.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO namespace_list (namespace) VALUES ("fred")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// We need to unsure that we remove the namespace from the model list.
	// Otherwise, the db will be recreated on the next call to GetDB.
	err = ctrlDB.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM namespace_list WHERE namespace = "fred"`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.dbDeleter.DeleteDB("fred")
	c.Assert(err, gc.ErrorMatches, `.*"fred" not found`)

	_, err = s.dbGetter.GetDB("fred")
	c.Assert(err, gc.ErrorMatches, `.*"fred": database not found`)
	c.Assert(err, jc.ErrorIs, coredatabase.ErrDBNotFound)
}

type txnRunner struct {
	db *sql.DB
}

func (r *txnRunner) Txn(ctx context.Context, f func(context.Context, *sqlair.TX) error) error {
	return errors.Trace(database.Txn(ctx, sqlair.NewDB(r.db), f))
}

func (r *txnRunner) TxnWithPrecheck(ctx context.Context, precheck func(context.Context) error, f func(context.Context, *sqlair.TX) error) error {
	return errors.Trace(database.TxnWithPrecheck(ctx, sqlair.NewDB(r.db), precheck, f))
}

func (r *txnRunner) StdTxn(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	return errors.Trace(database.StdTxn(ctx, r.db, f))
}

type controllerConfigWatcher struct {
	changes chan struct{}
}

func (c controllerConfigWatcher) Changes() <-chan struct{} {
	return c.changes
}

func (c controllerConfigWatcher) Done() <-chan struct{} {
	panic("implement me")
}

func (c controllerConfigWatcher) Unsubscribe() {
	panic("implement me")
}

type clusterConfig struct{}

func (c clusterConfig) DBBindAddresses() (map[string]string, error) {
	return nil, nil
}
