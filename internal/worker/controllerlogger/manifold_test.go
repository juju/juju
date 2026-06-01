// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlogger_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	internallogger "github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/controllerlogger"
	loggerworker "github.com/juju/juju/internal/worker/logger"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	config controllerlogger.ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validManifoldConfig(c)
}

func validManifoldConfig(c *tc.C) controllerlogger.ManifoldConfig {
	return controllerlogger.ManifoldConfig{
		DomainServicesName: "domain-services",
		LoggerContext:      internallogger.WrapLoggoContext(loggo.NewContext(loggo.DEBUG)),
		Logger:             loggertesting.WrapCheckLog(c),
		Tag:                names.NewControllerAgentTag("0"),
		LoggingOverride:    "",
		UpdateAgentFunc:    func(string) error { return nil },
		GetControllerDomainServices: func(getter dependency.Getter, name string) (controllerlogger.ModelService, error) {
			return &stubModelService{}, nil
		},
		GetModelConfigService: func(getter dependency.Getter, name string, uuid coremodel.UUID) (controllerlogger.ModelConfigService, error) {
			return &stubModelConfigService{}, nil
		},
		NewWorker: func(cfg loggerworker.WorkerConfig) (worker.Worker, error) {
			return loggerworker.NewLogger(cfg)
		},
	}
}

func (s *ManifoldSuite) TestValidateEmptyDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilLoggerContext(c *tc.C) {
	s.config.LoggerContext = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilTag(c *tc.C) {
	s.config.Tag = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilGetControllerDomainServices(c *tc.C) {
	s.config.GetControllerDomainServices = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilGetModelConfigService(c *tc.C) {
	s.config.GetModelConfigService = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateSuccess(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	manifold := controllerlogger.Manifold(s.config)
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (s *ManifoldSuite) TestStartSuccess(c *tc.C) {
	manifold := controllerlogger.Manifold(s.config)
	getter := dt.StubGetter(map[string]interface{}{
		"domain-services": nil,
	})
	w, err := manifold.Start(context.Background(), getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.Not(tc.IsNil))
	worker.Stop(w)
}

func (s *ManifoldSuite) TestStartGetControllerDomainServicesError(c *tc.C) {
	s.config.GetControllerDomainServices = func(getter dependency.Getter, name string) (controllerlogger.ModelService, error) {
		return nil, errors.New("boom")
	}
	manifold := controllerlogger.Manifold(s.config)
	getter := dt.StubGetter(map[string]interface{}{
		"domain-services": nil,
	})
	_, err := manifold.Start(context.Background(), getter)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartGetModelConfigServiceError(c *tc.C) {
	s.config.GetModelConfigService = func(getter dependency.Getter, name string, uuid coremodel.UUID) (controllerlogger.ModelConfigService, error) {
		return nil, errors.New("no config")
	}
	manifold := controllerlogger.Manifold(s.config)
	getter := dt.StubGetter(map[string]interface{}{
		"domain-services": nil,
	})
	_, err := manifold.Start(context.Background(), getter)
	c.Assert(err, tc.ErrorMatches, "no config")
}

// Stubs

type stubModelService struct{}

func (s *stubModelService) GetControllerModelUUID(_ context.Context) (coremodel.UUID, error) {
	return "deadbeef-0000-0000-0000-000000000000", nil
}

type stubModelConfigService struct{}

func (s *stubModelConfigService) ModelConfig(_ context.Context) (*config.Config, error) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey:          "controller",
		config.TypeKey:          "ec2",
		config.UUIDKey:          "deadbeef-0000-0000-0000-000000000000",
		config.LoggingConfigKey: "<root>=DEBUG",
	})
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *stubModelConfigService) Watch(_ context.Context) (watcher.StringsWatcher, error) {
	return newStubStringsWatcher(), nil
}

type stubStringsWatcher struct {
	ch   chan []string
	done chan struct{}
}

func newStubStringsWatcher() *stubStringsWatcher {
	sw := &stubStringsWatcher{
		ch:   make(chan []string, 1),
		done: make(chan struct{}),
	}
	sw.ch <- []string{config.LoggingConfigKey}
	return sw
}

func (s *stubStringsWatcher) Changes() watcher.StringsChannel {
	return s.ch
}

func (s *stubStringsWatcher) Kill() {
	select {
	case <-s.done:
	default:
		close(s.done)
		close(s.ch)
	}
}

func (s *stubStringsWatcher) Wait() error { <-s.done; return nil }
func (s *stubStringsWatcher) Err() error  { return nil }
