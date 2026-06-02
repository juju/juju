// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlogger_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	internallogger "github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/controllerlogger"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite

	context corelogger.LoggerContext
	service *stubWorkerModelConfigService
	config  controllerlogger.Config
	mu      sync.Mutex
	updated string
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &WorkerSuite{})
}

func (s *WorkerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.context = internallogger.WrapLoggoContext(loggo.NewContext(loggo.DEBUG))
	s.service = newStubWorkerModelConfigService("<root>=INFO")
	s.updated = ""
	s.config = controllerlogger.Config{
		Context:        s.context,
		ModelConfigSvc: s.service,
		Tag:            names.NewControllerAgentTag("0"),
		Logger:         loggertesting.WrapCheckLog(c),
		UpdateAgentFunc: func(v string) error {
			s.mu.Lock()
			s.updated = v
			s.mu.Unlock()
			return nil
		},
	}
}

func (s *WorkerSuite) TestValidateMissingModelConfigService(c *tc.C) {
	s.config.ModelConfigSvc = nil
	w, err := controllerlogger.NewWorker(c.Context(), s.config)
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "missing model config service not valid")
}

func (s *WorkerSuite) TestInitialState(c *tc.C) {
	w, err := controllerlogger.NewWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Assert(worker.Stop(w), tc.ErrorIsNil) }()

	s.waitLoggingInfo(c, "<root>=INFO")
	c.Check(s.updatedValue(), tc.Equals, "<root>=INFO")
}

func (s *WorkerSuite) TestConfigOverride(c *tc.C) {
	s.config.Override = "test=TRACE"
	w, err := controllerlogger.NewWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Assert(worker.Stop(w), tc.ErrorIsNil) }()

	s.waitLoggingInfo(c, "<root>=WARNING;test=TRACE")
	c.Check(s.updatedValue(), tc.Equals, "test=TRACE")
}

func (s *WorkerSuite) TestWatchedLoggingConfigChange(c *tc.C) {
	w, err := controllerlogger.NewWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Assert(worker.Stop(w), tc.ErrorIsNil) }()

	s.waitLoggingInfo(c, "<root>=INFO")
	s.service.setLoggingConfig(c, "module=DEBUG")
	s.service.emit(c, []string{"other-key"})
	// Irrelevant changes must not reconfigure logging.
	time.Sleep(20 * time.Millisecond)
	c.Check(s.context.Config().String(), tc.Equals, "<root>=INFO")

	s.service.emit(c, []string{config.LoggingConfigKey})
	s.waitLoggingInfo(c, "<root>=WARNING;module=DEBUG")
	c.Check(s.updatedValue(), tc.Equals, "module=DEBUG")
}

func (s *WorkerSuite) updatedValue() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updated
}

func (s *WorkerSuite) waitLoggingInfo(c *tc.C, expected string) {
	timeout := time.After(testhelpers.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for logging info to change")
		case <-time.After(10 * time.Millisecond):
			loggerInfo := s.context.Config().String()
			if loggerInfo != expected {
				continue
			}
			return
		}
	}
}

type stubWorkerModelConfigService struct {
	current string
	watcher *stubWorkerStringsWatcher
}

func newStubWorkerModelConfigService(loggingConfig string) *stubWorkerModelConfigService {
	return &stubWorkerModelConfigService{
		current: loggingConfig,
		watcher: newStubWorkerStringsWatcher(),
	}
}

func (s *stubWorkerModelConfigService) ModelConfig(_ context.Context) (*config.Config, error) {
	return config.New(config.UseDefaults, map[string]any{
		config.NameKey:          "controller",
		config.TypeKey:          "ec2",
		config.UUIDKey:          "deadbeef-0000-0000-0000-000000000000",
		config.LoggingConfigKey: s.current,
	})
}

func (s *stubWorkerModelConfigService) Watch(_ context.Context) (watcher.StringsWatcher, error) {
	return s.watcher, nil
}

func (s *stubWorkerModelConfigService) setLoggingConfig(_ *tc.C, value string) {
	s.current = value
}

func (s *stubWorkerModelConfigService) emit(c *tc.C, keys []string) {
	s.watcher.emit(c, keys)
}

type stubWorkerStringsWatcher struct {
	changes chan []string
	done    chan struct{}
}

func newStubWorkerStringsWatcher() *stubWorkerStringsWatcher {
	return &stubWorkerStringsWatcher{
		changes: make(chan []string, 10),
		done:    make(chan struct{}),
	}
}

func (s *stubWorkerStringsWatcher) Changes() watcher.StringsChannel {
	return s.changes
}

func (s *stubWorkerStringsWatcher) Kill() {
	select {
	case <-s.done:
	default:
		close(s.done)
		close(s.changes)
	}
}

func (s *stubWorkerStringsWatcher) Wait() error {
	<-s.done
	return nil
}

func (s *stubWorkerStringsWatcher) emit(c *tc.C, keys []string) {
	select {
	case s.changes <- keys:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("timed out sending watcher changes")
	}
}
