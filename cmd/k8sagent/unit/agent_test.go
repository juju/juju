// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/voyeur"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/k8sagent/unit"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/logsender"
)

type k8sUnitAgentSuite struct {
	coretesting.BaseSuite

	rootDir string
	dataDir string
}

var _ = gc.Suite(&k8sUnitAgentSuite{})

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

func (s *k8sUnitAgentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.rootDir = c.MkDir()
	s.dataDir = filepath.Join(s.rootDir, "/var/lib/juju")
}

func (s *k8sUnitAgentSuite) prepareAgentConf(c *gc.C, appName string) string {
	agentDir := filepath.Join(s.dataDir, "agents", names.NewApplicationTag(appName).String())
	err := os.MkdirAll(agentDir, 0700)
	c.Assert(err, gc.IsNil)
	fPath := filepath.Join(agentDir, "agent.conf")
	err = ioutil.WriteFile(fPath, []byte(fmt.Sprintf(agentConfigContents, appName)), 0600)
	c.Assert(err, gc.IsNil)
	return fPath
}

func (s *k8sUnitAgentSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func (s *k8sUnitAgentSuite) TestParseSuccess(c *gc.C) {
	_ = s.prepareAgentConf(c, "wordpress")

	a, err := unit.NewForTest(nil, s.newBufferedLogWriter(), nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(a, []string{
		"--data-dir", s.dataDir,
		"--application-name", "wordpress",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(a.DataDir(), gc.Equals, s.dataDir)
	c.Check(a.ApplicationName(), gc.Equals, "wordpress")
}

func (s *k8sUnitAgentSuite) TestParseMissing(c *gc.C) {
	uc, err := unit.NewForTest(nil, s.newBufferedLogWriter(), nil)
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(uc, []string{
		"--data-dir", "jc",
	})

	c.Assert(err, gc.ErrorMatches, "--application-name option must be set")
}

func (s *k8sUnitAgentSuite) TestParseNonsense(c *gc.C) {
	for _, args := range [][]string{
		{"--application-name", "wordpress/0"},
		{"--application-name", "wordpress/seventeen"},
		{"--application-name", "wordpress/-32"},
		{"--application-name", "wordpress/wild/9"},
		{"--application-name", "20"},
	} {
		a, err := unit.NewForTest(nil, s.newBufferedLogWriter(), nil)
		c.Assert(err, jc.ErrorIsNil)

		err = cmdtesting.InitCommand(a, append(args, "--data-dir", "jc"))
		c.Check(err, gc.ErrorMatches, `--application-name option expects "<application>" argument`)
	}
}

func (s *k8sUnitAgentSuite) TestParseUnknown(c *gc.C) {
	a, err := unit.NewForTest(nil, s.newBufferedLogWriter(), nil)
	c.Assert(err, jc.ErrorIsNil)

	err = cmdtesting.InitCommand(a, []string{
		"--application-name", "wordpress",
		"thundering typhoons",
	})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
}

func (s *k8sUnitAgentSuite) TestChangeConfig(c *gc.C) {
	config := FakeAgentConfig{}
	configChanged := voyeur.NewValue(true)

	a, err := unit.NewForTest(nil, s.newBufferedLogWriter(), configChanged)
	c.Assert(err, jc.ErrorIsNil)
	a.SetAgentConf(config)
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

	err = a.ChangeConfig(mutate)
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
