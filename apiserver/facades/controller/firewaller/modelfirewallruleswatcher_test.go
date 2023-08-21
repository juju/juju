// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/apiserver/facades/controller/firewaller/mocks"
	"github.com/juju/juju/environs/config"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ModelFirewallRulesWatcherSuite{})

type ModelFirewallRulesWatcherSuite struct {
	st *mocks.MockState
}

func (s *ModelFirewallRulesWatcherSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = mocks.NewMockState(ctrl)

	return ctrl
}

func cfg(c *gc.C, in map[string]interface{}) *config.Config {
	attrs := coretesting.FakeConfig().Merge(in)
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func mockNotifyWatcher(ctrl *gomock.Controller) (*mocks.MockNotifyWatcher, chan struct{}) {
	ch := make(chan struct{})
	watcher := mocks.NewMockNotifyWatcher(ctrl)
	watcher.EXPECT().Changes().Return(ch).MinTimes(1)
	watcher.EXPECT().Wait().AnyTimes()
	watcher.EXPECT().Kill().AnyTimes()
	watcher.EXPECT().Stop().AnyTimes()
	return watcher, ch
}

func (s *ModelFirewallRulesWatcherSuite) TestInitial(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	watcher, notifyCh := mockNotifyWatcher(ctrl)
	defer close(notifyCh)
	s.st.EXPECT().WatchForModelConfigChanges().Return(watcher)

	s.st.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.st)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- struct{}{}
	wc.AssertChanges(1)
}

func (s *ModelFirewallRulesWatcherSuite) TestConfigChange(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	watcher, notifyCh := mockNotifyWatcher(ctrl)
	defer close(notifyCh)

	s.st.EXPECT().WatchForModelConfigChanges().Return(watcher)

	s.st.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)
	s.st.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "192.168.0.0/24"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.st)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- struct{}{}
	wc.AssertChanges(1)

	// Config change
	notifyCh <- struct{}{}
	wc.AssertChanges(1)
}

func (s *ModelFirewallRulesWatcherSuite) TestIrrelevantConfigChange(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	watcher, notifyCh := mockNotifyWatcher(ctrl)
	defer close(notifyCh)

	s.st.EXPECT().WatchForModelConfigChanges().Return(watcher)

	s.st.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)
	s.st.EXPECT().ModelConfig(gomock.Any()).Return(cfg(c, map[string]interface{}{config.SSHAllowKey: "0.0.0.0/0"}), nil)

	w, err := firewaller.NewModelFirewallRulesWatcher(s.st)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewNotifyWatcherC(c, w)

	// Initial event
	notifyCh <- struct{}{}
	wc.AssertChanges(1)

	// Config change
	notifyCh <- struct{}{}
	wc.AssertNoChange()
}
