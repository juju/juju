// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater_test

import (
	"time"

	// "github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher/watchertest"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/auditconfigupdater"
	"github.com/juju/juju/worker/workertest"
)

type updaterSuite struct {
	jujutesting.BaseSuite
}

var _ = gc.Suite(&updaterSuite{})

var ding = struct{}{}

func (s *updaterSuite) TestWorker(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	output := make(chan auditlog.Config)
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

	w, err := auditconfigupdater.New(&source, initial, factory, output)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.cfg["auditing-enabled"] = true
	configChanged <- ding

	var newConfig auditlog.Config
	select {
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("took too long to send change")
	case newConfig = <-output:
	}

	c.Assert(newConfig.Enabled, gc.Equals, true)
	c.Assert(newConfig.CaptureAPIArgs, gc.Equals, false)
	c.Assert(newConfig.ExcludeMethods, gc.DeepEquals, set.NewStrings())
	c.Assert(newConfig.Target, gc.Equals, auditlog.AuditLog(&fakeTarget))
	c.Assert(calls, gc.HasLen, 1)
}

func (s *updaterSuite) TestIgnoresIrrelevantChange(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	output := make(chan auditlog.Config)
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

	w, err := auditconfigupdater.New(&source, initial, factory, output)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// No change.
	configChanged <- ding

	select {
	case <-time.After(jujutesting.ShortWait):
	case <-output:
		c.Fatalf("irrelevant change shouldn't have triggered audit config change")
	}

	source.cfg = makeControllerConfig(true, false)
	configChanged <- ding

	var newConfig auditlog.Config
	select {
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("change ignored")
	case newConfig = <-output:
	}

	c.Assert(newConfig.Enabled, gc.Equals, true)
	c.Assert(newConfig.CaptureAPIArgs, gc.Equals, false)
	c.Assert(newConfig.ExcludeMethods, gc.DeepEquals, set.NewStrings())
	c.Assert(newConfig.Target, gc.Equals, auditlog.AuditLog(&fakeTarget))
	c.Assert(calls, gc.HasLen, 1)

	// No change.
	configChanged <- ding

	select {
	case <-time.After(jujutesting.ShortWait):
	case <-output:
		c.Fatalf("subsequent change shouldn't have triggered audit config change")
	}
}

func (s *updaterSuite) TestKeepsLogFileWhenAuditingDisabled(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	output := make(chan auditlog.Config)
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
	w, err := auditconfigupdater.New(&source, initial, nil, output)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.cfg = makeControllerConfig(false, false)
	configChanged <- ding

	var newConfig auditlog.Config
	select {
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("change ignored")
	case newConfig = <-output:
	}

	c.Assert(newConfig.Enabled, gc.Equals, false)
	c.Assert(newConfig.Target, gc.Equals, initial.Target)
}

func (s *updaterSuite) TestKeepsLogFileWhenEnabled(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	output := make(chan auditlog.Config)
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
	w, err := auditconfigupdater.New(&source, initial, nil, output)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.cfg = makeControllerConfig(true, false)
	configChanged <- ding

	var newConfig auditlog.Config
	select {
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("change ignored")
	case newConfig = <-output:
	}

	c.Assert(newConfig.Enabled, gc.Equals, true)
	c.Assert(newConfig.Target, gc.Equals, initial.Target)
}

func (s *updaterSuite) TestChangingExcludeMethod(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	output := make(chan auditlog.Config)
	initial := auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("Pink.Floyd"),
		Target:         &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false, "Pink.Floyd"),
	}

	w, err := auditconfigupdater.New(&source, initial, nil, output)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.cfg = makeControllerConfig(true, false, "Pink.Floyd", "Led.Zeppelin")
	configChanged <- ding

	var newConfig auditlog.Config
	select {
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("change ignored")
	case newConfig = <-output:
	}

	c.Assert(newConfig.ExcludeMethods, gc.DeepEquals, set.NewStrings("Pink.Floyd", "Led.Zeppelin"))

	source.cfg = makeControllerConfig(true, false, "Led.Zeppelin")
	configChanged <- ding

	select {
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("change ignored")
	case newConfig = <-output:
	}

	c.Assert(newConfig.ExcludeMethods, gc.DeepEquals, set.NewStrings("Led.Zeppelin"))
}

func (s *updaterSuite) TestChangingCaptureArgs(c *gc.C) {
	configChanged := make(chan struct{}, 1)
	output := make(chan auditlog.Config)
	initial := auditlog.Config{
		Enabled:        true,
		CaptureAPIArgs: false,
		Target:         &apitesting.FakeAuditLog{},
	}
	source := configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, false, "Pink.Floyd"),
	}

	w, err := auditconfigupdater.New(&source, initial, nil, output)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	source.cfg = makeControllerConfig(true, true)
	configChanged <- ding

	var newConfig auditlog.Config
	select {
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("change ignored")
	case newConfig = <-output:
	}

	c.Assert(newConfig.CaptureAPIArgs, gc.Equals, true)
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

type configSource struct {
	stub    testing.Stub
	watcher *watchertest.NotifyWatcher
	cfg     controller.Config
}

func (s *configSource) WatchControllerConfig() state.NotifyWatcher {
	s.stub.AddCall("WatchControllerConfig")
	return s.watcher
}

func (s *configSource) ControllerConfig() (controller.Config, error) {
	s.stub.AddCall("ControllerConfig")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.cfg, nil
}
