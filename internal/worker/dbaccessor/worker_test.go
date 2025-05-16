// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/database/dqlite"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	trackedDB *MockTrackedDB
}

func TestWorkerSuite(t *stdtesting.T) { tc.Run(t, &workerSuite{}) }
func (s *workerSuite) TestKilledGetDBErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbDone := make(chan struct{})
	s.expectClock()
	s.expectTrackedDBUpdateNodeAndKill(dbDone)

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.IsExistingNode().Return(true, nil).Times(1)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.IsLoopbackPreferred().Return(false)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	// We may or may not get this call.
	mgrExp.SetClusterToLocalNode(gomock.Any()).Return(nil).AnyTimes()

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("simulates absent config for initial check"))

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	w := s.newWorker(c)
	defer func() {
		close(dbDone)
		workertest.DirtyKill(c, w)
	}()
	dbw := w.(*dbWorker)
	ensureStartup(c, dbw)

	w.Kill()

	_, err := dbw.GetDB("anything")
	c.Assert(err, tc.ErrorIs, database.ErrDBAccessorDying)
}

func (s *workerSuite) TestStartupTimeoutSingleControllerReconfigure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.IsExistingNode().Return(true, nil).Times(2)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(false, nil).Times(3)
	mgrExp.IsLoopbackPreferred().Return(false).Times(2)
	mgrExp.WithTLSOption().Return(nil, nil)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)
	mgrExp.SetClusterToLocalNode(gomock.Any()).Return(nil)

	// App gets started, we time out waiting, then we close it.
	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(context.DeadlineExceeded)
	appExp.Close().Return(nil)

	s.expectNoConfigChanges()

	// We always check for actionable configuration at startup.
	// Simulate config with just this node as a member.
	// We should reconfigure as just us and restart the worker.
	s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{"0": "10.6.6.6:1234"}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, dependency.ErrBounce)
}

func (s *workerSuite) TestStartupTimeoutMultipleControllerRetry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil).Times(2)
	mgrExp.IsExistingNode().Return(true, nil).Times(2)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(false, nil).Times(4)

	// We expect 1 attempt to start and 2 attempts to reconfigure.
	mgrExp.IsLoopbackPreferred().Return(false).Times(3)

	// We expect 2 attempts to start.
	mgrExp.WithTLSOption().Return(nil, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil).Times(2)
	mgrExp.WithTracingOption().Return(nil).Times(2)

	// App gets started, we time out waiting, then we close it both times.
	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(context.DeadlineExceeded).Times(2)
	appExp.Close().Return(nil).Times(2)

	s.expectNoConfigChanges()

	// Config shows we're in a cluster and not alone.
	// Since we can't start, and we are not invoking a back-stop scenario,
	// We can't reason about our state.
	s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
		"0": "10.6.6.6",
		"1": "10.6.6.7",
	}, nil)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)
	dbw := w.(*dbWorker)

	// At this point, the Dqlite node is not started.
	// The worker is waiting for legitimate server detail messages.
	select {
	case <-dbw.dbReady:
		c.Fatal("Dqlite node should not be started yet.")
	case <-time.After(testing.ShortWait):
	}
}

func (s *workerSuite) TestStartupNotExistingNodeThenCluster(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbDone := make(chan struct{})
	s.expectClock()
	s.expectTrackedDBUpdateNodeAndKill(dbDone)

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.IsExistingNode().Return(false, nil).Times(4)
	mgrExp.WithAddressOption("10.6.6.6").Return(nil)
	mgrExp.WithClusterOption([]string{"10.6.6.7"}).Return(nil)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTLSOption().Return(nil, nil)
	mgrExp.WithTracingOption().Return(nil)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(false, nil)

	// Expects 1 attempt to start and 2 attempts to reconfigure.
	mgrExp.IsLoopbackPreferred().Return(false).Times(3)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.dbApp.EXPECT().Handover(gomock.Any()).Return(nil)

	// First time though, there is no config, then we get 2
	// notifications for changes on disk.
	ch := make(chan struct{})
	s.expectConfigChanges(ch)

	gomock.InOrder(
		// Just us; no reconfiguration.
		// This is the check at start-up.
		s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
			"0": "10.6.6.6",
		}, nil),
		// Not address for us; no reconfiguration.
		s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
			"1": "10.6.6.7",
			"2": "10.6.6.8",
		}, nil),
		// Legit cluster config; away we go.
		s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
			"0": "10.6.6.6",
			"1": "10.6.6.7",
		}, nil),
	)

	w := s.newWorker(c)
	defer func() {
		close(dbDone)
		workertest.CleanKill(c, w)
	}()
	dbw := w.(*dbWorker)

	// This is the first config change, which has incomplete cluster config.
	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for config change to be processed")
	}

	// At this point, the Dqlite node is not started.
	// The worker is waiting for legitimate server detail messages.
	select {
	case <-dbw.dbReady:
		c.Fatal("Dqlite node should not be started yet.")
	case <-time.After(testing.ShortWait):
	}

	// This is the final config change, which has complete cluster config.
	// The node should start subsequently.
	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for config change to be processed")
	}

	ensureStartup(c, dbw)

	// This is a supplementary test of the report.
	s.client.EXPECT().Leader(gomock.Any()).Return(&dqlite.NodeInfo{
		ID:      1,
		Address: "10.10.1.1",
	}, nil)
	report := w.(interface{ Report() map[string]any }).Report()
	c.Assert(report, MapHasKeys, []string{
		"leader",
		"leader-id",
		"leader-role",
	})
}

func (s *workerSuite) TestWorkerStartupExistingNode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbDone := make(chan struct{})
	s.expectClock()
	s.expectTrackedDBUpdateNodeAndKill(dbDone)

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)

	// If this is an existing node, we do not invoke the address or cluster
	// options, but if the node is not as bootstrapped, we do assume it is
	// part of a cluster, and uses the TLS option.
	// These multiple calls occur during startup, the first config check,
	// and at shutdown when checking for handover.
	mgrExp.IsExistingNode().Return(true, nil).MinTimes(1)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(false, nil).MinTimes(1)
	mgrExp.IsLoopbackPreferred().Return(false).MinTimes(1)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTLSOption().Return(nil, nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	// Config shows we're in a cluster and not alone,
	// so we don't attempt to reconfigure ourselves.
	s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
		"0": "10.6.6.6",
		"1": "10.6.6.7",
	}, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()
	s.dbApp.EXPECT().Handover(gomock.Any()).Return(nil)

	w := s.newWorker(c)
	defer func() {
		close(dbDone)
		workertest.CleanKill(c, w)
	}()

	ensureStartup(c, w.(*dbWorker))
}

func (s *workerSuite) TestWorkerStartupExistingNodeWithLoopbackPreferred(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbDone := make(chan struct{})
	s.expectClock()
	s.expectTrackedDBUpdateNodeAndKill(dbDone)

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)

	// If this is an existing node, we do not invoke the address or cluster
	// options, but if the node is not as bootstrapped, we do assume it is
	// part of a cluster, and does not use the TLS option.
	// These multiple calls occur during startup, the first config check,
	// and at shutdown when checking for handover.
	// We don't expect a handover, because we're not rebinding.
	mgrExp.IsExistingNode().Return(true, nil).MinTimes(1)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).MinTimes(1)
	mgrExp.IsLoopbackPreferred().Return(false).MinTimes(1)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()
	s.expectNoConfigChanges()

	// This would be the case on K8s, where we have no counterpart units.
	s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("not found"))

	w := s.newWorker(c)
	defer func() {
		close(dbDone)
		workertest.CleanKill(c, w)
	}()

	ensureStartup(c, w.(*dbWorker))
}

func (s *workerSuite) TestWorkerStartupAsBootstrapNodeSingleServerNoRebind(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbDone := make(chan struct{})
	s.expectClock()
	s.expectTrackedDBUpdateNodeAndKill(dbDone)

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil).MinTimes(1)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(4)
	mgrExp.IsLoopbackPreferred().Return(false).Times(3)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	s.expectNodeStartupAndShutdown()

	// First time though, there is no config, then we get 2
	// notifications for changes on disk.
	ch := make(chan struct{})
	s.expectConfigChanges(ch)

	gomock.InOrder(
		s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("not there")),
		// Just us; no reconfiguration.
		s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
			"0": "10.6.6.6",
		}, nil),
		// Not address for us; no reconfiguration.
		s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
			"1": "10.6.6.7",
			"2": "10.6.6.8",
		}, nil),
	)

	w := s.newWorker(c)
	defer func() {
		close(dbDone)
		workertest.CleanKill(c, w)
	}()
	dbw := w.(*dbWorker)

	ensureStartup(c, dbw)

	// At this point we have started successfully.
	// Push the config change notifications.
	// None of the cause reconfiguration.
	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for config change to be processed")
	}

	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for config change to be processed")
	}
}

func (s *workerSuite) TestWorkerStartupAsBootstrapNodeThenReconfigure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbDone := make(chan struct{})
	s.expectClock()
	s.expectTrackedDBUpdateNodeAndKill(dbDone)

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil).Times(2)
	mgrExp.IsLoopbackPreferred().Return(false).Times(2)
	gomock.InOrder(
		mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2),
		// This is the check at shutdown.
		mgrExp.IsLoopbackBound(gomock.Any()).Return(false, nil))

	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	// These are the expectations around reconfiguring
	// the cluster and local node.
	mgrExp.ClusterServers(gomock.Any()).Return([]dqlite.NodeInfo{
		{
			ID:      3297041220608546238,
			Address: "127.0.0.1:17666",
			Role:    0,
		},
	}, nil)
	mgrExp.SetClusterServers(gomock.Any(), []dqlite.NodeInfo{
		{
			ID:      3297041220608546238,
			Address: "10.6.6.6:17666",
			Role:    0,
		},
	}).Return(nil)
	mgrExp.SetNodeInfo(dqlite.NodeInfo{
		ID:      3297041220608546238,
		Address: "10.6.6.6:17666",
		Role:    0,
	}).Return(nil)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	// Although the shut-down check for IsLoopbackBound returns false,
	// this call to shut-down is actually run before reconfiguring the node.
	// When the loop exits, the node is already set to nil.
	s.expectNodeStartupAndShutdown()

	// First time though, there is no config, then we get a
	// notification for a change on disk.
	ch := make(chan struct{})
	s.expectConfigChanges(ch)

	gomock.InOrder(
		s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("not there")),
		s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{
			"0": "10.6.6.6",
			"1": "10.6.6.7",
			"2": "10.6.6.8",
		}, nil),
	)

	w := s.newWorker(c)
	defer func() {
		close(dbDone)
		err := workertest.CheckKilled(c, w)
		c.Assert(err, tc.ErrorIs, dependency.ErrBounce)
	}()
	dbw := w.(*dbWorker)

	ensureStartup(c, dbw)

	// At this point we have started successfully.
	// Push a config change notification to simulate a move into HA.
	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for config change to be processed")
	}
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	return s.newWorkerWithDB(c, s.trackedDB)
}

func (s *workerSuite) TestWorkerStartupAsBootstrapNodeThenReconfigureWithLoopbackPreferred(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dbDone := make(chan struct{})
	s.expectClock()
	s.expectTrackedDBUpdateNodeAndKill(dbDone)

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTracingOption().Return(nil)

	// If this is a loopback preferred node, we do not invoke the TLS or
	// cluster options.
	mgrExp.IsExistingNode().Return(true, nil).Times(2)
	mgrExp.IsLoopbackPreferred().Return(true).Times(2)
	mgrExp.IsLoopbackBound(gomock.Any()).Return(true, nil).Times(2)

	// Ensure that we expect a clean startup and shutdown.
	s.expectNodeStartupAndShutdown()

	// First time though, there is no config, then we get a
	// notification for a change on disk.
	ch := make(chan struct{})
	s.expectConfigChanges(ch)

	gomock.InOrder(
		s.clusterConfig.EXPECT().DBBindAddresses().Return(nil, errors.New("not there")),
		s.clusterConfig.EXPECT().DBBindAddresses().Return(map[string]string{"0": "10.6.6.6"}, nil),
	)

	s.client.EXPECT().Cluster(gomock.Any()).Return(nil, nil)

	w := s.newWorker(c)
	defer func() {
		close(dbDone)
		// We do want a clean kill here, because we're not rebinding.
		workertest.CleanKill(c, w)
	}()
	dbw := w.(*dbWorker)

	ensureStartup(c, dbw)

	// At this point we have started successfully.
	// Push a config change notification to simulate a move into HA.
	// Notice the absence of expected calls to [Set]ClusterServer
	// and SetNodeInfo methods, because we eschew reconfiguration when
	// loopback binding is preferred.
	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for config change to be processed")
	}
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.trackedDB = NewMockTrackedDB(ctrl)

	return ctrl
}

// expectTrackedDBKillUpdateNode encompasses:
// - Use by the controller node service to update the node info.
// - Kill and wait upon termination of the worker.
// The input channel is used to ensure that the runner call to Wait does not
// return until we are ready.
// The Kill expectation is soft, because this can be done via parent catacomb,
// rather than a direct call.
func (s *workerSuite) expectTrackedDBUpdateNodeAndKill(done chan struct{}) {
	s.trackedDB.EXPECT().Txn(gomock.Any(), gomock.Any())
	s.trackedDB.EXPECT().Kill().AnyTimes()
	s.trackedDB.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	})
}
