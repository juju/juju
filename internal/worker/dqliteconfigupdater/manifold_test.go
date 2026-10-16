// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqliteconfigupdater

import (
	"context"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
)

func TestManifoldSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &ManifoldSuite{})
	})
}

type ManifoldSuite struct {
	baseSuite
}

func (s *ManifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		RuntimeConfigPath:  filepath.Join(c.MkDir(), controllerruntimeconfig.Filename),
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldSuite) TestValidateConfig(c *tc.C) {
	config := s.getConfig(c)
	c.Check(config.Validate(), tc.ErrorIsNil)

	config.DomainServicesName = ""
	c.Check(config.Validate(), tc.ErrorIs, coreerrors.NotValid)

	config = s.getConfig(c)
	config.RuntimeConfigPath = ""
	c.Check(config.Validate(), tc.ErrorIs, coreerrors.NotValid)

	config = s.getConfig(c)
	config.Logger = nil
	c.Check(config.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

var expectedInputs = []string{"domain-services"}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(Manifold(s.getConfig(c)).Inputs, tc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	// The manifold start resolves the controller config service from the
	// domain-services dependency and builds the worker over a real
	// runtime.conf delivery cache path.
	runtimeConfigPath := filepath.Join(c.MkDir(), controllerruntimeconfig.Filename)
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimeConfigPath, validRuntimeConfig())
	c.Assert(err, tc.ErrorIsNil)

	w, err := Manifold(ManifoldConfig{
		DomainServicesName: "domain-services",
		RuntimeConfigPath:  runtimeConfigPath,
		Logger:             loggertesting.WrapCheckLog(c),
	}).Start(c.Context(), dt.StubGetter(map[string]any{
		"domain-services": &stubControllerDomainServices{
			controllerConfigService: controllerconfigservice.NewWatchableService(
				stubControllerConfigState{}, stubWatcherFactory{},
			),
		},
	}))
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestStartMissingDomainServices(c *tc.C) {
	w, err := Manifold(s.getConfig(c)).Start(c.Context(), dt.StubGetter(nil))
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "getting controller config service: unexpected resource name: domain-services")
}

// validRuntimeConfig returns a controller runtime config that passes
// validation, so the worker's read and change closures bind a real
// delivery-cache file.
func validRuntimeConfig() controllerruntimeconfig.ControllerRuntimeConfig {
	return controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:          "0",
		ControllerUUID:        "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:   "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:               "/var/lib/juju",
		LogDir:                "/var/log/juju",
		SocketDir:             "/var/snap/juju/common/sockets",
		SharedAgentDir:        "/var/snap/juju/common",
		APIPort:               17070,
		AgentPassword:         "agent-password",
		QueryTracingEnabled:   true,
		QueryTracingThreshold: time.Second,
		DqliteBusyTimeout:     2 * time.Second,
		CACert:                "ca-cert-pem",
		CAPrivateKey:          "ca-private-key-pem",
		ControllerCert:        "controller-cert-pem",
		ControllerPrivateKey:  "controller-private-key-pem",
	}
}

// stubControllerDomainServices satisfies the services.ControllerDomainServices
// interface for the manifold start, providing a real watchable controller
// config service.
type stubControllerDomainServices struct {
	services.ControllerDomainServices

	controllerConfigService *controllerconfigservice.WatchableService
}

func (s *stubControllerDomainServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return s.controllerConfigService
}

// stubControllerConfigState satisfies the controller config state methods
// used by WatchControllerConfig.
type stubControllerConfigState struct {
	controllerconfigservice.State
}

func (stubControllerConfigState) NamespacesForWatchControllerConfig() []string {
	return []string{"controller_config"}
}

func (stubControllerConfigState) AllKeysQuery() string {
	return "SELECT key, value FROM controller_config"
}

// stubWatcherFactory satisfies the watcher factory interface used by
// WatchControllerConfig.
type stubWatcherFactory struct {
	controllerconfigservice.WatcherFactory
}

func (stubWatcherFactory) NewNamespaceWatcher(
	_ context.Context, _ eventsource.NamespaceQuery, _ string,
	_ eventsource.FilterOption, _ ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	return watchertest.NewMockStringsWatcher(make(chan []string)), nil
}
