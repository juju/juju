// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
)

var _ = tc.Suite(&ModelFirewallRulesWatcherSuite{})

type ModelFirewallRulesWatcherSuite struct {
	modelConfigService *MockModelConfigService
}

func (s *ModelFirewallRulesWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfigService = NewMockModelConfigService(ctrl)

	return ctrl
}

func cfg(c *tc.C, in map[string]interface{}) *config.Config {
	attrs := coretesting.FakeConfig().Merge(in)
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *ModelFirewallRulesWatcherSuite) TestInitial(c *tc.C) {
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

func (s *ModelFirewallRulesWatcherSuite) TestConfigChange(c *tc.C) {
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

func (s *ModelFirewallRulesWatcherSuite) TestIrrelevantConfigChange(c *tc.C) {
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
