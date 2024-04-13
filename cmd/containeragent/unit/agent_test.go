// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/voyeur"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/containeragent/unit"
	utilsmocks "github.com/juju/juju/cmd/containeragent/utils/mocks"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	k8sconstants "github.com/juju/juju/internal/provider/caas/kubernetes/provider/constants"
	"github.com/juju/juju/internal/worker/logsender"
	jnames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type containerUnitAgentSuite struct {
	testing.BaseSuite

	rootDir          string
	dataDir          string
	fileReaderWriter *utilsmocks.MockFileReaderWriter
	environment      *utilsmocks.MockEnvironment
	cmd              unit.ContainerUnitAgentTest
}

var _ = gc.Suite(&containerUnitAgentSuite{})

var agentConfigContents = `
# format 2.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
tag: unit-%s-0
datadir: /home/user/.local/share/juju/local
logdir: /var/log/juju-user-local
upgradedToVersion: 2.9-beta1
apiaddresses:
- localhost:17070
apiport: 17070
`[1:]

func (s *containerUnitAgentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.rootDir = c.MkDir()
	s.dataDir = filepath.Join(s.rootDir, "/var/lib/juju")
	err := os.MkdirAll(s.dataDir, 0700)
	c.Assert(err, gc.IsNil)
}

func (s *containerUnitAgentSuite) TearDownTest(c *gc.C) {
	s.dataDir = ""
	s.fileReaderWriter = nil
}

func (s *containerUnitAgentSuite) setupCommand(c *gc.C, configChangedVal *voyeur.Value) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.fileReaderWriter = utilsmocks.NewMockFileReaderWriter(ctrl)
	s.environment = utilsmocks.NewMockEnvironment(ctrl)
	s.cmd = unit.NewForTest(nil, s.newBufferedLogWriter(), configChangedVal, s.fileReaderWriter, s.environment)
	return ctrl
}

func (s *containerUnitAgentSuite) prepareAgentConf(c *gc.C, appName string) string {
	fPath := filepath.Join(s.dataDir, k8sconstants.TemplateFileNameAgentConf)
	err := os.WriteFile(fPath, []byte(fmt.Sprintf(agentConfigContents, appName)), 0600)
	c.Assert(err, gc.IsNil)
	return fPath
}

func (s *containerUnitAgentSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func (s *containerUnitAgentSuite) TestParseSuccess(c *gc.C) {
	ctrl := s.setupCommand(c, nil)
	defer ctrl.Finish()

	_ = s.prepareAgentConf(c, "wordpress")

	toolsDir := filepath.Join(s.dataDir, "tools", "unit-wordpress-0")
	gomock.InOrder(
		s.environment.EXPECT().ExpandEnv("$PATH:test-bin").Return("old-path:test-bin"),
		s.environment.EXPECT().Setenv("PATH", "old-path:test-bin").Return(nil),
		s.environment.EXPECT().Unsetenv("DELETE").Return(nil),
		s.fileReaderWriter.EXPECT().RemoveAll(toolsDir).Return(nil),
		s.fileReaderWriter.EXPECT().MkdirAll(toolsDir, os.FileMode(0755)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.ContainerAgent)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join("/usr/bin", jnames.JujuExec)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join("/usr/bin", jnames.JujuIntrospect)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.Jujuc)).Return(nil),
		s.environment.EXPECT().Getenv("JUJU_CONTAINER_NAMES").Return("a,b,c"),
	)

	err := cmdtesting.InitCommand(s.cmd, []string{
		"--data-dir", s.dataDir,
		"--charm-modified-version", "10",
		"--append-env", "PATH=$PATH:test-bin",
		"--append-env", "DELETE=",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.cmd.DataDir(), gc.Equals, s.dataDir)
	c.Assert(s.cmd.Tag().String(), jc.DeepEquals, `unit-wordpress-0`)
	c.Assert(s.cmd.CurrentConfig().Controller().String(), jc.DeepEquals, `controller-deadbeef-1bad-500d-9000-4b1d0d06f00d`)
	c.Assert(s.cmd.CurrentConfig().Model().String(), jc.DeepEquals, `model-deadbeef-0bad-400d-8000-4b1d0d06f00d`)
	c.Assert(s.cmd.CharmModifiedVersion(), gc.Equals, 10)
	c.Assert(s.cmd.GetContainerNames(), jc.DeepEquals, []string{"a", "b", "c"})
}

func (s *containerUnitAgentSuite) TestParseSuccessNoContainer(c *gc.C) {
	ctrl := s.setupCommand(c, nil)
	defer ctrl.Finish()

	_ = s.prepareAgentConf(c, "wordpress")

	toolsDir := filepath.Join(s.dataDir, "tools", "unit-wordpress-0")
	gomock.InOrder(
		s.environment.EXPECT().ExpandEnv("$PATH:test-bin").Return("old-path:test-bin"),
		s.environment.EXPECT().Setenv("PATH", "old-path:test-bin").Return(nil),
		s.environment.EXPECT().Unsetenv("DELETE").Return(nil),
		s.fileReaderWriter.EXPECT().RemoveAll(toolsDir).Return(nil),
		s.fileReaderWriter.EXPECT().MkdirAll(toolsDir, os.FileMode(0755)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.ContainerAgent)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join("/usr/bin", jnames.JujuExec)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join("/usr/bin", jnames.JujuIntrospect)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.Jujuc)).Return(nil),
		s.environment.EXPECT().Getenv("JUJU_CONTAINER_NAMES").Return(""),
	)

	err := cmdtesting.InitCommand(s.cmd, []string{
		"--data-dir", s.dataDir,
		"--charm-modified-version", "10",
		"--append-env", "PATH=$PATH:test-bin",
		"--append-env", "DELETE=",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.cmd.DataDir(), gc.Equals, s.dataDir)
	c.Assert(s.cmd.Tag().String(), jc.DeepEquals, `unit-wordpress-0`)
	c.Assert(s.cmd.CurrentConfig().Controller().String(), jc.DeepEquals, `controller-deadbeef-1bad-500d-9000-4b1d0d06f00d`)
	c.Assert(s.cmd.CurrentConfig().Model().String(), jc.DeepEquals, `model-deadbeef-0bad-400d-8000-4b1d0d06f00d`)
	c.Assert(s.cmd.CharmModifiedVersion(), gc.Equals, 10)
	c.Assert(len(s.cmd.GetContainerNames()), gc.Equals, 0)
}

func (s *containerUnitAgentSuite) TestParseUnknown(c *gc.C) {
	ctrl := s.setupCommand(c, nil)
	defer ctrl.Finish()

	err := cmdtesting.InitCommand(s.cmd, []string{
		"thundering typhoons",
	})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
}

func (s *containerUnitAgentSuite) TestChangeConfig(c *gc.C) {
	config := FakeAgentConfig{}
	configChanged := voyeur.NewValue(true)

	ctrl := s.setupCommand(c, configChanged)
	defer ctrl.Finish()

	s.cmd.SetAgentConf(config)
	var mutateCalled bool
	mutate := func(config agent.ConfigSetter) error {
		mutateCalled = true
		return nil
	}

	configChangedCh := make(chan bool)
	watcher := configChanged.Watch()
	watcher.Next() // consume initial event
	go func() {
		configChangedCh <- watcher.Next()
	}()

	err := s.cmd.ChangeConfig(mutate)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mutateCalled, jc.IsTrue)
	select {
	case result := <-configChangedCh:
		c.Check(result, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for config changed signal")
	}
}

func (s *containerUnitAgentSuite) TestEnsureAgentConf(c *gc.C) {
	dataDir := c.MkDir()
	params := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: dataDir,
		},
		Tag:                    names.NewUnitTag("app/0"),
		UpgradedToVersion:      jujuversion.Current,
		Password:               "password",
		CACert:                 "cacert",
		APIAddresses:           []string{"localhost:1235"},
		Nonce:                  "nonce",
		Controller:             testing.ControllerTag,
		Model:                  testing.ModelTag,
		AgentLogfileMaxSizeMB:  150,
		AgentLogfileMaxBackups: 4,
	}
	templateConfig, err := agent.NewAgentConfig(params)
	c.Assert(err, jc.ErrorIsNil)
	templateBytes, err := templateConfig.Render()
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(path.Join(dataDir, "template-agent.conf"), templateBytes, 0644)
	c.Assert(err, jc.ErrorIsNil)

	ac := agentconf.NewAgentConf(dataDir)
	err = unit.EnsureAgentConf(ac)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the agent.conf was seeded from the template-agent.conf.
	agentConfBytes, err := os.ReadFile(path.Join(dataDir, "agents", "unit-app-0", "agent.conf"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(agentConfBytes), gc.Equals, string(templateBytes))

	// Change the agent.conf to be different than the template-agent.conf
	c.Assert(ac.CurrentConfig().OldPassword(), gc.Equals, "password")
	err = ac.ChangeConfig(func(cs agent.ConfigSetter) error {
		cs.SetOldPassword("password2")
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ac.CurrentConfig().OldPassword(), gc.Equals, "password2")

	// Start a new "agent" and make sure it has password2.
	ac2 := agentconf.NewAgentConf(dataDir)
	err = unit.EnsureAgentConf(ac2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ac2.CurrentConfig().OldPassword(), gc.Equals, "password2")
}

type FakeConfig struct {
	agent.ConfigSetter
	values map[string]string
}

func (FakeConfig) LogDir() string {
	return filepath.FromSlash("/var/log/juju/")
}

func (FakeConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}

func (f FakeConfig) Value(key string) string {
	if f.values == nil {
		return ""
	}
	return f.values[key]
}

type FakeAgentConfig struct {
	agentconf.AgentConf
	values map[string]string
}

func (FakeAgentConfig) ReadConfig(string) error { return nil }

func (a FakeAgentConfig) CurrentConfig() agent.Config {
	return FakeConfig{values: a.values}
}

func (FakeAgentConfig) ChangeConfig(mutate agent.ConfigMutator) error {
	return mutate(FakeConfig{})
}

func (FakeAgentConfig) CheckArgs([]string) error { return nil }
