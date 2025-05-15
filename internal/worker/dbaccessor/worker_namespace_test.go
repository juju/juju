// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/testhelpers"
)

type namespaceSuite struct {
	dbBaseSuite
}

var _ = tc.Suite(&namespaceSuite{})

func (s *namespaceSuite) TestEnsureNamespaceForController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := &dbWorker{
		dbApp: s.dbApp,
	}

	err := w.ensureNamespace(c.Context(), database.ControllerNS)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *namespaceSuite) TestEnsureNamespaceForModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(false)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err := dbw.ensureNamespace(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, database.ErrDBNotFound)

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestEnsureNamespaceForModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(false)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	ctx, cancel := context.WithTimeout(c.Context(), testhelpers.LongWait)
	defer cancel()

	dbw := s.startWorker(c, ctx)
	defer workertest.DirtyKill(c, dbw)

	err := dbw.ensureNamespace(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, dbw)
}

func (s *namespaceSuite) TestEnsureNamespaceForModelLoopbackPreferred(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(true)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(1)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	ctx, cancel := context.WithTimeout(c.Context(), testhelpers.LongWait)
	defer cancel()

	dbw := s.startWorker(c, ctx)
	defer workertest.DirtyKill(c, dbw)

	err := dbw.ensureNamespace(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, dbw)
}

func (s *namespaceSuite) TestEnsureNamespaceForModelWithCache(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(false)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	ctx, cancel := context.WithTimeout(c.Context(), testhelpers.LongWait)
	defer cancel()

	var (
		attempt int
		num     int64
	)
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		attempt++

		stmt := "INSERT INTO namespace_list (namespace) VALUES (?);"
		result, err := tx.ExecContext(ctx, stmt, "foo")
		if err != nil {
			return err
		}

		num, err = result.RowsAffected()
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(num, tc.Equals, int64(1))

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err = dbw.ensureNamespace(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	// The second query will be cached.
	err = dbw.ensureNamespace(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(attempt, tc.Equals, 1)

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) TestCloseDatabaseForController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(false)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	ctx, cancel := context.WithTimeout(c.Context(), testhelpers.LongWait)
	defer cancel()

	dbw := s.startWorker(c, ctx)
	defer workertest.DirtyKill(c, dbw)

	err := dbw.deleteDatabase(c.Context(), database.ControllerNS)
	c.Assert(err, tc.ErrorMatches, "cannot delete controller database")

	workertest.CleanKill(c, dbw)
}

func (s *namespaceSuite) TestCloseDatabaseForModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(false)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	db, err := s.DBApp().Open(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	s.dbApp.EXPECT().Open(gomock.Any(), "foo").Return(db, nil)

	ctx, cancel := context.WithTimeout(c.Context(), testhelpers.LongWait)
	defer cancel()

	dbw := s.startWorker(c, ctx)
	defer workertest.DirtyKill(c, dbw)

	_, err = dbw.GetDB("foo")
	c.Assert(err, tc.ErrorIsNil)

	err = dbw.deleteDatabase(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, dbw)
}

func (s *namespaceSuite) TestCloseDatabaseForModelLoopbackPreferred(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(true)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(1)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	db, err := s.DBApp().Open(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	s.dbApp.EXPECT().Open(gomock.Any(), "foo").Return(db, nil)

	ctx, cancel := context.WithTimeout(c.Context(), testhelpers.LongWait)
	defer cancel()

	dbw := s.startWorker(c, ctx)
	defer workertest.DirtyKill(c, dbw)

	_, err = dbw.GetDB("foo")
	c.Assert(err, tc.ErrorIsNil)

	err = dbw.deleteDatabase(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, dbw)
}

func (s *namespaceSuite) TestCloseDatabaseForUnknownModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsLoopbackPreferred().Return(false)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)
	defer workertest.DirtyKill(c, w)

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	err := dbw.deleteDatabase(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	workertest.CleanKill(c, w)
}

func (s *namespaceSuite) startWorker(c *tc.C, ctx context.Context) *dbWorker {
	trackedWorkerDB := newWorkerTrackedDB(s.TxnRunner())

	w := s.newWorkerWithDB(c, trackedWorkerDB)

	var num int64
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO namespace_list (namespace) VALUES (?);"
		result, err := tx.ExecContext(ctx, stmt, "foo")
		if err != nil {
			return err
		}

		num, err = result.RowsAffected()
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(num, tc.Equals, int64(1))

	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	return dbw
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

func (w *workerTrackedDB) KillWithReason(reason error) {
	w.tomb.Kill(reason)
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
