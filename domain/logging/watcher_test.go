// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logging_test

import (
	"context"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/logging"
	"github.com/juju/juju/domain/logging/service"
	"github.com/juju/juju/domain/logging/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchLokiConfigSet(c *tc.C) {
	svc := s.setupService(c)

	watcher, err := svc.WatchLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Setting a new loki endpoint should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetLokiConfig(c.Context(), logging.LokiConfig{
			Endpoint:      "http://loki:3100/loki/api/v1/push",
			CACertificate: "ca-cert",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLokiConfigEndpointUpdate(c *tc.C) {
	svc := s.setupService(c)

	// Set an initial endpoint before starting the watcher.
	err := svc.SetLokiConfig(c.Context(), logging.LokiConfig{
		Endpoint:      "http://old-loki:3100/loki/api/v1/push",
		CACertificate: "old-ca",
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Updating the endpoint should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetLokiConfig(c.Context(), logging.LokiConfig{
			Endpoint:      "http://new-loki:3100/loki/api/v1/push",
			CACertificate: "old-ca",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLokiConfigCACertificateUpdate(c *tc.C) {
	svc := s.setupService(c)

	// Set an initial endpoint before starting the watcher.
	err := svc.SetLokiConfig(c.Context(), logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "old-ca",
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Updating only the CA certificate should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetLokiConfig(c.Context(), logging.LokiConfig{
			Endpoint:      "http://loki:3100/loki/api/v1/push",
			CACertificate: "new-ca",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLokiConfigDelete(c *tc.C) {
	svc := s.setupService(c)

	// Set an initial endpoint before starting the watcher.
	err := svc.SetLokiConfig(c.Context(), logging.LokiConfig{
		Endpoint:      "http://loki:3100/loki/api/v1/push",
		CACertificate: "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Deleting the endpoint should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.DeleteLokiConfig(c.Context())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLokiConfigNoChange(c *tc.C) {
	svc := s.setupService(c)

	watcher, err := svc.WatchLokiConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// No changes should result in no notifications.
	harness.AddTest(c, func(c *tc.C) {
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C) *service.WatchableService {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "logging_loki_config")

	return service.NewWatchableService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) {
			return factory(ctx)
		}),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}
