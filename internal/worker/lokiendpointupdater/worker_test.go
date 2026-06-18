// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lokiendpointupdater

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type workerSuite struct {
	testhelpers.IsolationSuite
	agent         *fakeAgent
	api           *fakeAPI
	configChanged *voyeur.Value
	config        WorkerConfig
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.agent = &fakeAgent{config: newAgentConfig(c)}
	s.api = &fakeAPI{
		watcher: &mockNotifyWatcher{changes: make(chan struct{})},
		config: logger.ControllerLokiConfig{
			Endpoint: "https://loki.example.com/loki/api/v1/push",
			CACert:   "ca-cert",
		},
	}
	s.configChanged = voyeur.NewValue(true)
	s.config = WorkerConfig{
		Agent:              s.agent,
		API:                s.api,
		AgentConfigChanged: s.configChanged,
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

func (s *workerSuite) TestValidate(c *tc.C) {
	c.Check(WorkerConfig{}.Validate(), tc.ErrorMatches, "missing agent not valid")

	config := s.config
	config.API = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing api not valid")

	config = s.config
	config.AgentConfigChanged = nil
	c.Check(config.Validate(), tc.ErrorMatches, "nil AgentConfigChanged not valid")

	config = s.config
	config.Logger = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing logger not valid")

	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *workerSuite) TestSetUpPersistsInitialConfigAndStartsWatcher(c *tc.C) {
	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	w, err := worker.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(w, tc.Equals, s.api.watcher)
	c.Check(s.agent.config.LokiEndpoint(), tc.Equals, "https://loki.example.com/loki/api/v1/push")
	c.Check(s.agent.config.LokiCACert(), tc.Equals, "ca-cert")
	c.Check(s.agent.changeCalls, tc.Equals, 1)
	c.Check(s.api.getTag, tc.Equals, s.agent.config.Tag())
	c.Check(s.api.watchTag, tc.Equals, s.agent.config.Tag())
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestSetUpDoesNotWriteUnchangedConfig(c *tc.C) {
	s.agent.config.SetLokiConfig(s.api.config.Endpoint, &s.api.config.CACert, s.api.config.InsecureSkipVerify)
	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	_, err := worker.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.agent.changeCalls, tc.Equals, 0)
	assertNoConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsChangedConfig(c *tc.C) {
	oldCACert := "old-ca"
	s.agent.config.SetLokiConfig("https://old-loki.example.com/loki/api/v1/push", &oldCACert, nil)
	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.agent.config.LokiEndpoint(), tc.Equals, "https://loki.example.com/loki/api/v1/push")
	c.Check(s.agent.config.LokiCACert(), tc.Equals, "ca-cert")
	c.Check(s.agent.changeCalls, tc.Equals, 1)
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsEmptyConfigForLogSinkMode(c *tc.C) {
	oldCACert := "old-ca"
	s.agent.config.SetLokiConfig("https://old-loki.example.com/loki/api/v1/push", &oldCACert, nil)
	s.api.config = logger.ControllerLokiConfig{}
	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.agent.config.LokiEndpoint(), tc.Equals, "")
	c.Check(s.agent.config.LokiCACert(), tc.Equals, "")
	c.Check(s.agent.changeCalls, tc.Equals, 1)
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsEmptyConfigForLokiConfigNotFound(c *tc.C) {
	oldCACert := "old-ca"
	s.agent.config.SetLokiConfig("https://old-loki.example.com/loki/api/v1/push", &oldCACert, nil)
	s.api.getErr = &params.Error{
		Code:    params.CodeNotFound,
		Message: "loki config not found",
	}
	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.agent.config.LokiEndpoint(), tc.Equals, "")
	c.Check(s.agent.config.LokiCACert(), tc.Equals, "")
	c.Check(s.agent.changeCalls, tc.Equals, 1)
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandleGetError(c *tc.C) {
	s.api.getErr = errors.New("boom")
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())

	c.Assert(err, tc.ErrorMatches, "getting controller loki config: boom")
	c.Check(s.agent.changeCalls, tc.Equals, 0)
}

func (s *workerSuite) TestHandleChangeConfigError(c *tc.C) {
	s.agent.changeErr = errors.New("boom")
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())

	c.Assert(err, tc.ErrorMatches, "updating agent loki config: boom")
	c.Check(s.agent.config.LokiEndpoint(), tc.Equals, "")
	c.Check(s.agent.config.LokiCACert(), tc.Equals, "")
}

func (s *workerSuite) newUpdater(c *tc.C) *lokiEndpointUpdater {
	c.Assert(s.config.Validate(), tc.ErrorIsNil)
	return &lokiEndpointUpdater{
		config: s.config,
		tag:    s.agent.config.Tag(),
	}
}

type fakeAgent struct {
	config      agent.ConfigSetterWriter
	changeErr   error
	changeCalls int
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return a.config
}

func (a *fakeAgent) ChangeConfig(mutator agent.ConfigMutator) error {
	if a.changeErr != nil {
		return a.changeErr
	}
	a.changeCalls++
	return mutator(a.config)
}

type fakeAPI struct {
	config   logger.ControllerLokiConfig
	getErr   error
	watchErr error
	watcher  watcher.NotifyWatcher
	getTag   names.Tag
	watchTag names.Tag
}

func (a *fakeAPI) GetControllerLokiConfig(_ context.Context, tag names.Tag) (logger.ControllerLokiConfig, error) {
	a.getTag = tag
	return a.config, a.getErr
}

func (a *fakeAPI) WatchControllerLokiConfig(_ context.Context, tag names.Tag) (watcher.NotifyWatcher, error) {
	a.watchTag = tag
	return a.watcher, a.watchErr
}

type mockNotifyWatcher struct {
	changes chan struct{}
}

func (m *mockNotifyWatcher) Kill() {}

func (m *mockNotifyWatcher) Wait() error {
	return nil
}

func (m *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return m.changes
}

var _ watcher.NotifyWatcher = (*mockNotifyWatcher)(nil)

func newAgentConfig(c *tc.C) agent.ConfigSetterWriter {
	cfg, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: c.MkDir(),
			LogDir:  c.MkDir(),
		},
		Jobs:              []model.MachineJob{model.JobHostUnits},
		UpgradedToVersion: semversion.MustParse("4.0.0"),
		Tag:               names.NewMachineTag("0"),
		Password:          "password",
		Nonce:             "nonce",
		Controller:        coretesting.ControllerTag,
		Model:             coretesting.ModelTag,
		APIAddresses:      []string{"127.0.0.1:17070"},
		CACert:            "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func watchConfigChanged(value *voyeur.Value) <-chan bool {
	watcher := value.Watch()
	watcher.Next()
	ch := make(chan bool, 1)
	go func() {
		defer watcher.Close()
		ch <- watcher.Next()
	}()
	return ch
}

func assertConfigChanged(c *tc.C, ch <-chan bool) {
	select {
	case changed := <-ch:
		c.Check(changed, tc.IsTrue)
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("timed out waiting for config changed signal")
	}
}

func assertNoConfigChanged(c *tc.C, ch <-chan bool) {
	select {
	case <-ch:
		c.Fatalf("unexpected config changed signal")
	case <-time.After(testhelpers.ShortWait):
	}
}
