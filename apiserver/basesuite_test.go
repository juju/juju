// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/testserver"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/modelcache"
	"github.com/juju/juju/worker/multiwatcher"
)

type baseSuite struct {
	statetesting.StateSuite

	controller *cache.Controller

	cfg apiserver.ServerConfig
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)

	multiWatcherWorker, err := multiwatcher.NewWorker(multiwatcher.Config{
		Logger:               loggo.GetLogger("test"),
		Backing:              state.NewAllWatcherBacking(s.StatePool),
		PrometheusRegisterer: noopRegisterer{},
	})
	// The worker itself is a coremultiwatcher.Factory.
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, multiWatcherWorker) })

	initialized := gate.NewLock()
	modelCache, err := modelcache.NewWorker(modelcache.Config{
		StatePool:            s.StatePool,
		Hub:                  pubsub.NewStructuredHub(nil),
		InitializedGate:      initialized,
		Logger:               loggo.GetLogger("modelcache"),
		WatcherFactory:       multiWatcherWorker.WatchController,
		PrometheusRegisterer: noopRegisterer{},
		Cleanup:              func() {},
	}.WithDefaultRestartStrategy())
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, modelCache) })

	select {
	case <-initialized.Unlocked():
	case <-time.After(testing.LongWait):
		c.Fatalf("model cache not initialized after %s", testing.LongWait)
	}
	err = modelcache.ExtractCacheController(modelCache, &s.controller)
	c.Assert(err, jc.ErrorIsNil)

	s.cfg = testserver.DefaultServerConfig(c, s.Clock)
	s.cfg.Controller = s.controller
}

func (s *baseSuite) newServer(c *gc.C) *api.Info {
	server := testserver.NewServerWithConfig(c, s.StatePool, s.cfg)
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, server.APIServer)
		server.HTTPServer.Close()
	})
	server.Info.ModelTag = s.Model.ModelTag()
	return server.Info
}

func (s *baseSuite) openAPIWithoutLogin(c *gc.C, info0 *api.Info) api.Connection {
	info := *info0
	info.Tag = nil
	info.Password = ""
	info.SkipLogin = true
	info.Macaroons = nil
	st, err := api.Open(&info, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { _ = st.Close() })
	return st
}

// derivedSuite is just here to test newServer is clean.
type derivedSuite struct {
	baseSuite
}

var _ = gc.Suite(&derivedSuite{})

func (s *derivedSuite) TestNewServer(c *gc.C) {
	_ = s.newServer(c)
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}
