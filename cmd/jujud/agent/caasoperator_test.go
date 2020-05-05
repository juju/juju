// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/agent"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	gc "gopkg.in/check.v1"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	coretesting "github.com/juju/juju/testing"
	jujuworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/logsender"
)

type CAASOperatorSuite struct {
	coretesting.BaseSuite

	rootDir string
}

var _ = gc.Suite(&CAASOperatorSuite{})

func newExecClient(modelName string) (exec.Executor, error) {
	return nil, nil
}

func (s *CAASOperatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.rootDir = c.MkDir()
}

func (s *CAASOperatorSuite) dataDir() string {
	return filepath.Join(s.rootDir, "/var/lib/juju")
}

func (s *CAASOperatorSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func (s *CAASOperatorSuite) TestParseSuccess(c *gc.C) {
	// Now init actually reads the agent configuration file.
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = newExecClient
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(a, []string{
		"--data-dir", s.dataDir(),
		"--application-name", "wordpress",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(a.AgentConf.DataDir(), gc.Equals, s.dataDir())
	c.Check(a.ApplicationName, gc.Equals, "wordpress")
}

func (s *CAASOperatorSuite) TestParseMissing(c *gc.C) {
	uc, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = newExecClient
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(uc, []string{
		"--data-dir", "jc",
	})

	c.Assert(err, gc.ErrorMatches, "--application-name option must be set")
}

func (s *CAASOperatorSuite) TestParseNonsense(c *gc.C) {
	for _, args := range [][]string{
		{"--application-name", "wordpress/0"},
		{"--application-name", "wordpress/seventeen"},
		{"--application-name", "wordpress/-32"},
		{"--application-name", "wordpress/wild/9"},
		{"--application-name", "20"},
	} {
		a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
			mc.NewExecClient = newExecClient
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)

		err = cmdtesting.InitCommand(a, append(args, "--data-dir", "jc"))
		c.Check(err, gc.ErrorMatches, `--application-name option expects "<application>" argument`)
	}
}

func (s *CAASOperatorSuite) TestParseUnknown(c *gc.C) {
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = newExecClient
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = cmdtesting.InitCommand(a, []string{
		"--application-name", "wordpress",
		"thundering typhoons",
	})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
}

func (s *CAASOperatorSuite) TestLogStderr(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)

	a := CaasOperatorAgent{
		AgentConf:       FakeAgentConfig{},
		ctx:             ctx,
		ApplicationName: "mysql",
		dead:            make(chan struct{}),
	}

	err = a.Init(nil)
	c.Assert(err, gc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsFalse)
}

var agentConfigContents = `
# format 2.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
tag: machine-0
datadir: /home/user/.local/share/juju/local
logdir: /var/log/juju-user-local
upgradedToVersion: 1.2.3
apiaddresses:
- localhost:17070
apiport: 17070
`[1:]

func (s *CAASOperatorSuite) TestRunCopiesConfigTemplate(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)
	dataDir := c.MkDir()
	agentDir := filepath.Join(dataDir, "agents", "application-mysql")
	err = os.MkdirAll(agentDir, 0700)
	c.Assert(err, gc.IsNil)
	templateFile := filepath.Join(agentDir, "template-agent.conf")

	err = ioutil.WriteFile(templateFile, []byte(agentConfigContents), 0600)
	c.Assert(err, gc.IsNil)

	a := &CaasOperatorAgent{
		AgentConf:       NewAgentConf(dataDir),
		ctx:             ctx,
		ApplicationName: "mysql",
		bufferedLogger:  s.newBufferedLogWriter(),
		dead:            make(chan struct{}),
	}

	dummy := jujuworker.NewSimpleWorker(func(stopCh <-chan struct{}) error {
		return jujuworker.ErrTerminateAgent
	})
	s.PatchValue(&CaasOperatorManifolds, func(config caasoperator.ManifoldsConfig) dependency.Manifolds {
		return dependency.Manifolds{"test": dependency.Manifold{
			Start: func(context dependency.Context) (worker.Worker, error) {
				return dummy, nil
			},
		}}
	})

	err = a.Init(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = a.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	agentConfig := a.CurrentConfig()
	c.Assert(agentConfig.Controller(), gc.Equals, names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00d"))
	addr, err := agentConfig.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.SameContents, []string{"localhost:17070"})
}

func (s *CAASOperatorSuite) TestChangeConfig(c *gc.C) {
	config := FakeAgentConfig{}
	configChanged := voyeur.NewValue(true)
	a := UnitAgent{
		AgentConf:        config,
		configChangedVal: configChanged,
	}

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

	err := a.ChangeConfig(mutate)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mutateCalled, jc.IsTrue)
	select {
	case result := <-configChangedCh:
		c.Check(result, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for config changed signal")
	}
}
