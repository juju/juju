// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"github.com/juju/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	workertest "github.com/juju/juju/worker/testing"
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

type stubWorkerFactory struct {
	*testing.Stub

	ReturnNewModelWorker func() (worker.Worker, error)
	ReturnNewWorker      worker.Worker
}

func newStubWorkerFactory(stub *testing.Stub) *stubWorkerFactory {
	factory := &stubWorkerFactory{Stub: stub}
	factory.ReturnNewWorker = &workertest.StubWorker{Stub: stub}
	factory.ReturnNewModelWorker = factory.newWorker
	return factory
}

func (s *stubWorkerFactory) NewModelWorker(st *state.State) func() (worker.Worker, error) {
	s.AddCall("NewModelWorker", st)
	s.NextErr() // Pop one off.

	return s.ReturnNewModelWorker
}

func (s *stubWorkerFactory) newWorker() (worker.Worker, error) {
	s.AddCall("newWorker")
	if err := s.NextErr(); err != nil {
		return nil, err
	}

	return s.ReturnNewWorker, nil
}
