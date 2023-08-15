// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/pubsub/apiserver"
)

type namespaceSuite struct {
	dbBaseSuite
}

var _ = gc.Suite(&namespaceSuite{})

func (s *namespaceSuite) TestEnsureNamespaceForController(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := &dbWorker{
		dbApp: s.dbApp,
	}

	err := w.ensureNamespace(database.ControllerNS)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *namespaceSuite) TestEnsureNamespaceForModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil).Times(1)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err := dbw.ensureNamespace("foo")
	c.Assert(errors.Is(err, errors.NotFound), jc.IsTrue)

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestEnsureNamespaceForModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	ctx, cancel := context.WithTimeout(context.Background(), testing.LongWait)
	defer cancel()

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO model_list (uuid) VALUES (?);"
		result, err := tx.ExecContext(ctx, stmt, "foo")
		c.Assert(err, jc.ErrorIsNil)

		num, err := result.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(num, gc.Equals, int64(1))

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err = dbw.ensureNamespace("foo")
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestEnsureNamespaceForModelWithCache(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	ctx, cancel := context.WithTimeout(context.Background(), testing.LongWait)
	defer cancel()

	var attempt int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		attempt++

		stmt := "INSERT INTO model_list (uuid) VALUES (?);"
		result, err := tx.ExecContext(ctx, stmt, "foo")
		c.Assert(err, jc.ErrorIsNil)

		num, err := result.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(num, gc.Equals, int64(1))

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err = dbw.ensureNamespace("foo")
	c.Assert(err, jc.ErrorIsNil)

	// The second query will be cached.
	err = dbw.ensureNamespace("foo")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(attempt, gc.Equals, 1)

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestCloseDatabaseForController(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	ctx, cancel := context.WithTimeout(context.Background(), testing.LongWait)
	defer cancel()

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO model_list (uuid) VALUES (?);"
		result, err := tx.ExecContext(ctx, stmt, "foo")
		c.Assert(err, jc.ErrorIsNil)

		num, err := result.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(num, gc.Equals, int64(1))

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err = dbw.closeDatabase(database.ControllerNS)
	c.Assert(err, gc.ErrorMatches, "cannot close controller database")

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestCloseDatabaseForModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	ctx, cancel := context.WithTimeout(context.Background(), testing.LongWait)
	defer cancel()

	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO model_list (uuid) VALUES (?);"
		result, err := tx.ExecContext(ctx, stmt, "foo")
		c.Assert(err, jc.ErrorIsNil)

		num, err := result.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(num, gc.Equals, int64(1))

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	_, err = dbw.GetDB("foo")
	c.Assert(err, jc.ErrorIsNil)

	err = dbw.closeDatabase("foo")
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestCloseDatabaseForUnknownModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err := dbw.closeDatabase("foo")
	c.Assert(err, gc.ErrorMatches, `stopping worker: worker "foo" not found`)

	workertest.CleanKill(c, w)
}

type workerTrackedDB struct {
	tomb tomb.Tomb
	db   database.TxnRunner
}

func newWorkerTrackedDB(db database.TxnRunner) *workerTrackedDB {
	w := &workerTrackedDB{
		db: db,
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *workerTrackedDB) loop() error {
	<-w.tomb.Dying()
	return tomb.ErrDying
}

func (w *workerTrackedDB) Kill() {
	w.tomb.Kill(nil)
}

func (w *workerTrackedDB) Wait() error {
	return w.tomb.Wait()
}

func (w *workerTrackedDB) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	return w.db.Txn(ctx, fn)
}

func (w *workerTrackedDB) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return w.db.StdTxn(ctx, fn)
}
