// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater_test

import (
	"reflect"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher/watchertest"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/auditconfigupdater"
)

type updaterSuite struct {
	jujutesting.BaseSuite
}

var _ = gc.Suite(&updaterSuite{})

var ding = struct{}{}

func (s *updaterSuite) TestWorker(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled: false,
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(false, false),
	}

	fakeTarget := apitesting.FakeAuditLog{}
	var calls []auditlog.Config
	factory := func(cfg auditlog.Config) auditlog.AuditLog {
		calls = append(calls, cfg)
		return &fakeTarget
	}

	w, err := auditconfigupdater.New(&source, initial, factory)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, false))
	configChanged <- ding

	newConfig := waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return cfg.Enabled
	})

	c.Assert(newConfig.Enabled, gc.Equals, true)
	c.Assert(newConfig.CaptureAPIArgs, gc.Equals, false)
	c.Assert(newConfig.ExcludeMethods, gc.DeepEquals, set.NewStrings())
	c.Assert(newConfig.Target, gc.Equals, auditlog.AuditLog(&fakeTarget))
	c.Assert(calls, gc.HasLen, 1)
}

func waitForConfig(c *gc.C, w worker.Worker, predicate func(auditlog.Config) bool) auditlog.Config {
	for a := jujutesting.LongAttempt.Start(); a.Next(); {
		config := getWorkerConfig(c, w)
		if predicate(config) {
			return config
		}
	}
	c.Fatalf("timed out waiting for matching config")
	return auditlog.Config{}
}

func (s *updaterSuite) TestKeepsLogFileWhenAuditingDisabled(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled: true,
		Target:  &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false),
	}

	// Passing a nil factory means we can be sure it didn't try to
	// create a new logfile.
	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(false, false))
	configChanged <- ding

	newConfig := waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return !cfg.Enabled
	})

	c.Assert(newConfig.Enabled, gc.Equals, false)
	c.Assert(newConfig.Target, gc.Equals, initial.Target)
}

func (s *updaterSuite) TestKeepsLogFileWhenEnabled(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled: false,
		Target:  &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(false, false),
	}

	// Passing a nil factory means we can be sure it didn't try to
	// create a new logfile.
	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, false))
	configChanged <- ding

	newConfig := waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return cfg.Enabled
	})

	c.Assert(newConfig.Enabled, gc.Equals, true)
	c.Assert(newConfig.Target, gc.Equals, initial.Target)
}

func (s *updaterSuite) TestChangingExcludeMethod(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("Pink.Floyd"),
		Target:         &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false, "Pink.Floyd"),
	}

	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, false, "Pink.Floyd", "Led.Zeppelin"))
	configChanged <- ding

	waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return reflect.DeepEqual(cfg.ExcludeMethods, set.NewStrings("Pink.Floyd", "Led.Zeppelin"))
	})

	source.setConfig(makeControllerConfig(true, false, "Led.Zeppelin"))
	configChanged <- ding

	waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return reflect.DeepEqual(cfg.ExcludeMethods, set.NewStrings("Led.Zeppelin"))
	})
}

func (s *updaterSuite) TestChangingCaptureArgs(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	initial := auditlog.Config{
		Enabled:        true,
		CaptureAPIArgs: false,
		Target:         &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false, "Pink.Floyd"),
	}

	w, err := auditconfigupdater.New(&source, initial, nil)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.setConfig(makeControllerConfig(true, true))
	configChanged <- ding

	waitForConfig(c, w, func(cfg auditlog.Config) bool {
		return cfg.CaptureAPIArgs
	})
}

func makeControllerConfig(auditEnabled bool, captureArgs bool, methods ...interface{}) controller.Config {
	result := map[string]interface{}{
		"other-setting":             "something",
		"auditing-enabled":          auditEnabled,
		"audit-log-capture-args":    captureArgs,
		"audit-log-exclude-methods": methods,
	}
	return result
}

func getWorkerConfig(c *gc.C, w worker.Worker) auditlog.Config {
	getter, ok := w.(interface {
		CurrentConfig() auditlog.Config
	})
	if !ok {
		c.Fatalf("worker %T doesn't expose CurrentConfig()", w)
	}
	return getter.CurrentConfig()
}

type configSource struct {
	mu      sync.Mutex
	stub    testing.Stub
	watcher *watchertest.NotifyWatcher
	cfg     controller.Config
}

func (s *configSource) WatchControllerConfig() state.NotifyWatcher {
	s.stub.AddCall("WatchControllerConfig")
	return s.watcher
}

func (s *configSource) ControllerConfig() (controller.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stub.AddCall("ControllerConfig")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.cfg, nil
}

func (s *configSource) setConfig(cfg controller.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
}
