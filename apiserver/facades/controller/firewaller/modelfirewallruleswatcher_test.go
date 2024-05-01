// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ModelFirewallRulesWatcherSuite{})

type ModelFirewallRulesWatcherSuite struct {
	modelConfigService *MockModelConfigService
}

func (s *ModelFirewallRulesWatcherSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfigService = NewMockModelConfigService(ctrl)

	return ctrl
}

func cfg(c *gc.C, in map[string]interface{}) *config.Config {
	attrs := coretesting.FakeConfig().Merge(in)
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func mockNotifyWatcher(ctrl *gomock.Controller) (*MockNotifyWatcher, chan struct{}) {
	ch := make(chan struct{})
	watcher := NewMockNotifyWatcher(ctrl)
	watcher.EXPECT().Changes().Return(ch).MinTimes(1)
	watcher.EXPECT().Wait().AnyTimes()
	watcher.EXPECT().Kill().AnyTimes()
	watcher.EXPECT().Stop().AnyTimes()
	return watcher, ch
}

func (s *ModelFirewallRulesWatcherSuite) TestInitial(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	notifyCh := make(chan []string)
	watcher := watchertest.NewMockStringsWatcher(notifyCh)
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.modelConfigService)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	wc := watchertest.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- []string{}
	wc.AssertChanges(testing.ShortWait)
}

func (s *ModelFirewallRulesWatcherSuite) TestConfigChange(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	notifyCh := make(chan []string)
	watcher := watchertest.NewMockStringsWatcher(notifyCh)
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "192.168.0.0/24"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.modelConfigService)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	wc := watchertest.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- []string{}
	wc.AssertChanges(testing.ShortWait)

	// Config change
	notifyCh <- []string{"ssh-allow"}
	wc.AssertChanges(testing.ShortWait)
}

func (s *ModelFirewallRulesWatcherSuite) TestIrrelevantConfigChange(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	notifyCh := make(chan []string)
	watcher := watchertest.NewMockStringsWatcher(notifyCh)
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.modelConfigService)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	wc := watchertest.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- []string{}
	wc.AssertChanges(testing.ShortWait)

	// Config change
	notifyCh <- []string{"ssh-allow"}
	wc.AssertNoChange()
}
