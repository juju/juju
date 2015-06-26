package agent

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
)

// This file contains bits of test infrastructure that are shared by
// the unit and machine agent tests.

type runner interface {
	Run(*cmd.Context) error
	Stop() error
}

// runWithTimeout runs an agent and waits
// for it to complete within a reasonable time.
func runWithTimeout(r runner) error {
	done := make(chan error)
	go func() {
		done <- r.Run(nil)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(coretesting.LongWait):
	}
	err := r.Stop()
	return fmt.Errorf("timed out waiting for agent to finish; stop error: %v", err)
}

func newDummyWorker() worker.Worker {
	return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
		<-stop
		return nil
	})
}

type FakeConfig struct {
	agent.Config
}

func (FakeConfig) LogDir() string {
	return filepath.FromSlash("/var/log/juju/")
}

func (FakeConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}

type FakeAgentConfig struct {
	AgentConf
}

func (FakeAgentConfig) ReadConfig(string) error { return nil }

func (FakeAgentConfig) CurrentConfig() agent.Config {
	return FakeConfig{}
}

func (FakeAgentConfig) CheckArgs([]string) error { return nil }
