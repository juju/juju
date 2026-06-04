// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing_test

import (
	"context"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/domain/tracing/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchWorkloadTracingConfigSet(c *tc.C) {
	svc := s.setupService(c)

	w, err := svc.WatchWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	h := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	h.AddTest(c, func(c *tc.C) {
		cfg := service.WorkloadTracingConfig{
			GRPCEndpoint:  "localhost:4317",
			CACertificate: "ca-cert",
		}
		err := svc.SetWorkloadTracingConfig(c.Context(), cfg)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	h.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchWorkloadTracingConfigDelete(c *tc.C) {
	svc := s.setupService(c)

	err := svc.SetWorkloadTracingConfig(c.Context(), service.WorkloadTracingConfig{
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	h := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	h.AddTest(c, func(c *tc.C) {
		err := svc.SetWorkloadTracingConfig(c.Context(), service.WorkloadTracingConfig{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	h.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchWorkloadTracingConfigNoChange(c *tc.C) {
	svc := s.setupService(c)

	cfg := service.WorkloadTracingConfig{
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "ca-cert",
	}
	err := svc.SetWorkloadTracingConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchWorkloadTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	h := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	h.AddTest(c, func(c *tc.C) {
		err := svc.SetWorkloadTracingConfig(c.Context(), cfg)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	h.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C) *service.WatchableService {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "workload_tracing_config")

	return service.NewWatchableService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) {
			return factory(ctx)
		}),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}
