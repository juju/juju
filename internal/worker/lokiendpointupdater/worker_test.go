// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lokiendpointupdater

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

var defaultLokiConfig = logger.ControllerLokiConfig{
	Endpoint: "https://loki.example.com/loki/api/v1/push",
	CACert:   "ca-cert",
}

type workerSuite struct {
	testhelpers.IsolationSuite

	agent         *MockAgent
	agentConfig   *MockConfig
	api           *MockLoggerAPI
	notifyWatcher *MockNotifyWatcher

	tag           names.Tag
	realConfig    agent.ConfigSetterWriter
	configChanged *voyeur.Value
	workerConfig  WorkerConfig
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.tag = names.NewMachineTag("0")
	s.realConfig = newAgentConfig(c)
	s.configChanged = voyeur.NewValue(true)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.api = NewMockLoggerAPI(ctrl)
	s.notifyWatcher = NewMockNotifyWatcher(ctrl)

	s.agentConfig.EXPECT().Tag().Return(s.tag).AnyTimes()
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()

	s.workerConfig = WorkerConfig{
		Agent:              s.agent,
		API:                s.api,
		AgentConfigChanged: s.configChanged,
		Logger:             loggertesting.WrapCheckLog(c),
	}

	c.Cleanup(func() {
		s.agent = nil
		s.agentConfig = nil
		s.api = nil
		s.notifyWatcher = nil
	})

	return ctrl
}

// expectCurrentConfigReads sets up the mock expectations for the read side of
// the current agent config. These are consulted in update() to decide whether
// the config needs to be written.
func (s *workerSuite) expectCurrentConfigReads(endpoint, caCert string, insecure *bool) {
	s.agentConfig.EXPECT().LokiEndpoint().Return(endpoint).AnyTimes()
	s.agentConfig.EXPECT().LokiCACert().Return(caCert).AnyTimes()
	s.agentConfig.EXPECT().LokiInsecureSkipVerify().Return(insecure).AnyTimes()
	s.agentConfig.EXPECT().LokiOrgID().Return("").AnyTimes()
}

// expectChangeConfig sets up ChangeConfig to invoke the mutator with a real
// agent.ConfigSetterWriter so that SetLokiConfig is exercised and the written
// values can be verified afterwards.
func (s *workerSuite) expectChangeConfig() {
	s.agent.EXPECT().ChangeConfig(gomock.Any()).
		DoAndReturn(func(mutator agent.ConfigMutator) error {
			return mutator(s.realConfig)
		})
}

// expectGetLokiConfig sets up the GetControllerLokiConfig expectation.
func (s *workerSuite) expectGetLokiConfig(config logger.ControllerLokiConfig, err error) {
	s.api.EXPECT().GetControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(config, err)
}

// expectWatch sets up the WatchControllerLokiConfig expectation.
func (s *workerSuite) expectWatch() {
	s.api.EXPECT().WatchControllerLokiConfig(gomock.Any(), gomock.Any()).
		Return(s.notifyWatcher, nil)
}

// --- WorkerConfig.Validate tests ---

func (s *workerSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Check(WorkerConfig{}.Validate(), tc.ErrorMatches, "missing agent not valid")

	config := s.workerConfig
	config.API = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing api not valid")

	config = s.workerConfig
	config.AgentConfigChanged = nil
	c.Check(config.Validate(), tc.ErrorMatches, "nil AgentConfigChanged not valid")

	config = s.workerConfig
	config.Logger = nil
	c.Check(config.Validate(), tc.ErrorMatches, "missing logger not valid")

	c.Check(s.workerConfig.Validate(), tc.ErrorIsNil)
}

func (s *workerSuite) TestSetUpPersistsInitialConfigAndStartsWatcher(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetLokiConfig(defaultLokiConfig, nil)
	s.expectCurrentConfigReads("", "", nil)
	s.expectChangeConfig()
	s.expectWatch()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	w, err := worker.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(w, tc.Equals, s.notifyWatcher)
	c.Check(s.realConfig.LokiEndpoint(), tc.Equals, "https://loki.example.com/loki/api/v1/push")
	c.Check(s.realConfig.LokiCACert(), tc.Equals, "ca-cert")
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestSetUpDoesNotWriteUnchangedConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetLokiConfig(defaultLokiConfig, nil)
	s.expectCurrentConfigReads(defaultLokiConfig.Endpoint, defaultLokiConfig.CACert, defaultLokiConfig.InsecureSkipVerify)
	s.expectWatch()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	_, err := worker.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.LokiEndpoint(), tc.Equals, "")
	c.Check(s.realConfig.LokiCACert(), tc.Equals, "")
	assertNoConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsChangedConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	oldCACert := "old-ca"
	s.expectGetLokiConfig(defaultLokiConfig, nil)
	s.expectCurrentConfigReads("https://old-loki.example.com/loki/api/v1/push", oldCACert, nil)
	s.expectChangeConfig()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.LokiEndpoint(), tc.Equals, "https://loki.example.com/loki/api/v1/push")
	c.Check(s.realConfig.LokiCACert(), tc.Equals, "ca-cert")
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsEmptyConfigForLogSinkMode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	oldCACert := "old-ca"
	s.expectGetLokiConfig(logger.ControllerLokiConfig{}, nil)
	s.expectCurrentConfigReads("https://old-loki.example.com/loki/api/v1/push", oldCACert, nil)
	s.expectChangeConfig()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.LokiEndpoint(), tc.Equals, "")
	c.Check(s.realConfig.LokiCACert(), tc.Equals, "")
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandlePersistsEmptyConfigForLokiConfigNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	oldCACert := "old-ca"
	// The API client restores params.CodeNotFound errors into a
	// coreerrors.NotFound-satisfying error before returning them to the
	// worker, so simulate that translated error here.
	s.expectGetLokiConfig(logger.ControllerLokiConfig{}, errors.NewNotFound(errors.New("loki config not found"), ""))
	s.expectCurrentConfigReads("https://old-loki.example.com/loki/api/v1/push", oldCACert, nil)
	s.expectChangeConfig()

	changeCh := watchConfigChanged(s.configChanged)
	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.realConfig.LokiEndpoint(), tc.Equals, "")
	c.Check(s.realConfig.LokiCACert(), tc.Equals, "")
	assertConfigChanged(c, changeCh)
}

func (s *workerSuite) TestHandleGetError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetLokiConfig(logger.ControllerLokiConfig{}, errors.New("boom"))
	s.expectCurrentConfigReads("", "", nil)

	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())

	c.Assert(err, tc.ErrorMatches, "getting controller loki config: boom")
	c.Check(s.realConfig.LokiEndpoint(), tc.Equals, "")
	c.Check(s.realConfig.LokiCACert(), tc.Equals, "")
}

func (s *workerSuite) TestHandleChangeConfigError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetLokiConfig(defaultLokiConfig, nil)
	s.expectCurrentConfigReads("", "", nil)
	s.agent.EXPECT().ChangeConfig(gomock.Any()).Return(errors.New("boom"))

	worker := s.newUpdater(c)

	err := worker.Handle(c.Context())

	c.Assert(err, tc.ErrorMatches, "updating agent loki config: boom")
	c.Check(s.realConfig.LokiEndpoint(), tc.Equals, "")
	c.Check(s.realConfig.LokiCACert(), tc.Equals, "")
}

func (s *workerSuite) newUpdater(c *tc.C) *lokiEndpointUpdater {
	c.Assert(s.workerConfig.Validate(), tc.ErrorIsNil)
	return &lokiEndpointUpdater{
		config: s.workerConfig,
		tag:    s.tag,
	}
}

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
