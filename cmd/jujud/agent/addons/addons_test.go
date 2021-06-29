// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addons_test

import (
	"runtime"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/addons"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/introspection"
)

type introspectionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&introspectionSuite{})

func (s *introspectionSuite) TestStartNonLinux(c *gc.C) {
	if runtime.GOOS == "linux" {
		c.Skip("testing for non-linux")
	}
	var started bool

	cfg := addons.IntrospectionConfig{
		WorkerFunc: func(_ introspection.Config) (worker.Worker, error) {
			started = true
			return nil, errors.New("shouldn't call start")
		},
	}

	err := addons.StartIntrospection(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(started, jc.IsFalse)
}

func (s *introspectionSuite) TestStartError(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	cfg := addons.IntrospectionConfig{
		Agent:         &dummyAgent{},
		NewSocketName: addons.DefaultIntrospectionSocketName,
		WorkerFunc: func(_ introspection.Config) (worker.Worker, error) {
			return nil, errors.New("boom")
		},
	}

	err := addons.StartIntrospection(cfg)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *introspectionSuite) TestStartSuccess(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}
	fake := &dummyWorker{
		done: make(chan struct{}),
	}

	config := dependency.EngineConfig{
		IsFatal:    agenterrors.IsFatal,
		WorstError: agenterrors.MoreImportantError,
		Clock:      clock.WallClock,
		Logger:     loggo.GetLogger("juju.worker.dependency"),
	}
	engine, err := dependency.NewEngine(config)
	c.Assert(err, jc.ErrorIsNil)

	cfg := addons.IntrospectionConfig{
		Agent:         &dummyAgent{},
		Engine:        engine,
		NewSocketName: func(tag names.Tag) string { return "bananas" },
		WorkerFunc: func(cfg introspection.Config) (worker.Worker, error) {
			fake.config = cfg
			return fake, nil
		},
	}

	err = addons.StartIntrospection(cfg)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fake.config.DepEngine, gc.Equals, engine)
	c.Check(fake.config.SocketName, gc.Equals, "bananas")

	// Stopping the engine causes the introspection worker to stop.
	engine.Kill()

	select {
	case <-fake.done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("worker did not get stopped")
	}
}

func (s *introspectionSuite) TestDefaultIntrospectionSocketName(c *gc.C) {
	name := addons.DefaultIntrospectionSocketName(names.NewMachineTag("42"))
	c.Assert(name, gc.Equals, "jujud-machine-42")
}

type dummyAgent struct {
	agent.Agent
}

func (*dummyAgent) CurrentConfig() agent.Config {
	return &dummyConfig{}
}

type dummyConfig struct {
	agent.Config
}

func (*dummyConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}

type dummyWorker struct {
	config introspection.Config
	done   chan struct{}
}

func (d *dummyWorker) Kill() {
	close(d.done)
}

func (d *dummyWorker) Wait() error {
	<-d.done
	return nil
}
