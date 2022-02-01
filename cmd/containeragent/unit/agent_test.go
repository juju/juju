// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows
// +build !windows

package unit_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/voyeur"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cmd/containeragent/unit"
	utilsmocks "github.com/juju/juju/cmd/containeragent/utils/mocks"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	jnames "github.com/juju/juju/juju/names"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/logsender"
)

type containerUnitAgentSuite struct {
	coretesting.BaseSuite

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
	err := ioutil.WriteFile(fPath, []byte(fmt.Sprintf(agentConfigContents, appName)), 0600)
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
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.JujuRun)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.JujuIntrospect)).Return(nil),
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
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.JujuRun)).Return(nil),
		s.fileReaderWriter.EXPECT().Symlink(gomock.Any(), filepath.Join(toolsDir, jnames.JujuIntrospect)).Return(nil),
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
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for config changed signal")
	}
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
