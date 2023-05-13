// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor_test

import (
	"context"
	sql "database/sql"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/database"
	"github.com/juju/juju/database/app"
	dqlite "github.com/juju/juju/database/dqlite"
	databasetesting "github.com/juju/juju/database/testing"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/dbaccessor"
)

type integrationSuite struct {
	dqliteAppIntegrationSuite

	dbManager coredatabase.DBManager
	worker    worker.Worker
}

var _ = gc.Suite(&integrationSuite{})

func (s *integrationSuite) SetUpSuite(c *gc.C) {
	s.DBSuite.SetUpSuite(c)

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

	logger := loggo.GetLogger("worker.dbaccessor.test")
	nodeManager := database.NewNodeManager(agentConfig, logger, coredatabase.NoopSlowQueryLogger{})

	w, err := dbaccessor.NewWorker(dbaccessor.WorkerConfig{
		NewApp: func(string, ...app.Option) (dbaccessor.DBApp, error) {
			return dbaccessor.WrapApp(s.DBApp()), nil
		},
		NewDBWorker:      dbaccessor.NewTrackedDBWorker,
		NodeManager:      nodeManager,
		MetricsCollector: dbaccessor.NewMetricsCollector(),
		Clock:            clock.WallClock,
		Logger:           logger,
		Hub:              pubsub.NewStructuredHub(nil),
		ControllerID:     agentConfig.Tag().Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.dbManager = w
	s.worker = w

	db, err := s.DBApp().Open(context.TODO(), coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)

	err = database.NewDBMigration(db, logger, schema.ControllerDDL()).Apply()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *integrationSuite) TearDownSuite(c *gc.C) {
	if dqlite.Enabled {
		workertest.CleanKill(c, s.worker)
	}

	s.dqliteAppIntegrationSuite.TearDownSuite(c)
}

func (s *integrationSuite) TestWorkerAccessingControllerDB(c *gc.C) {
	db, err := s.dbManager.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)
}

func (s *integrationSuite) TestWorkerAccessingUnknownDB(c *gc.C) {
	_, err := s.dbManager.GetDB("foo")
	c.Assert(err, gc.ErrorMatches, `.*namespace "foo" not found`)
}

func (s *integrationSuite) TestWorkerAccessingKnownDB(c *gc.C) {
	db, err := s.dbManager.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
	err = db.Txn(context.TODO(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO model_list (uuid) VALUES ("bar")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	db, err = s.dbManager.GetDB("bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)
}

func (s *integrationSuite) TestWorkerDeletingControllerDB(c *gc.C) {
	err := s.dbManager.DeleteDB(coredatabase.ControllerNS)
	c.Assert(err, gc.ErrorMatches, `.*cannot close controller database`)
}

func (s *integrationSuite) TestWorkerDeletingUnknownDB(c *gc.C) {
	_, err := s.dbManager.GetDB("foo")
	c.Assert(err, gc.ErrorMatches, `.*namespace "foo" not found`)
}

func (s *integrationSuite) TestWorkerDeletingKnownDB(c *gc.C) {
	db, err := s.dbManager.GetDB(coredatabase.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
	err = db.Txn(context.TODO(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO model_list (uuid) VALUES ("baz")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	db, err = s.dbManager.GetDB("baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	err = s.dbManager.DeleteDB("baz")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.dbManager.GetDB("baz")
	c.Assert(err, gc.ErrorMatches, `.*namespace "baz" not found`)
}

// integrationSuite defines a base suite for running integration tests against
// the dqlite database. It overrides the various methods to prevent the creation
// of a new database for each test.
type dqliteAppIntegrationSuite struct {
	databasetesting.DBSuite
}

func (s *dqliteAppIntegrationSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)

	// Note: we don't call s.DBSuite.TearDownSuite here because we don't want
	// to double close the dqlite app.
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
