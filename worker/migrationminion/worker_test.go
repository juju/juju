// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/workertest"
)

type Suite struct {
	coretesting.BaseSuite
	stub   *jujutesting.Stub
	client *stubMinionClient
	guard  *stubGuard
	agent  *stubAgent
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = new(jujutesting.Stub)
	s.client = newStubMinionClient(s.stub)
	s.guard = newStubGuard(s.stub)
	s.agent = newStubAgent()
}

func (s *Suite) TestStartAndStop(c *gc.C) {
	w, err := migrationminion.New(migrationminion.Config{
		Facade: s.client,
		Guard:  s.guard,
		Agent:  s.agent,
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "Watch")
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	s.client.watchErr = errors.New("boom")
	w, err := migrationminion.New(migrationminion.Config{
		Facade: s.client,
		Guard:  s.guard,
		Agent:  s.agent,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "setting up watcher: boom")
}

func (s *Suite) TestClosedWatcherChannel(c *gc.C) {
	close(s.client.watcher.changes)
	w, err := migrationminion.New(migrationminion.Config{
		Facade: s.client,
		Guard:  s.guard,
		Agent:  s.agent,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "watcher channel closed")
}

func (s *Suite) TestUnlockError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		Phase: migration.NONE,
	}
	s.guard.unlockErr = errors.New("squish")
	w, err := migrationminion.New(migrationminion.Config{
		Facade: s.client,
		Guard:  s.guard,
		Agent:  s.agent,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "squish")
	s.stub.CheckCallNames(c, "Watch", "Unlock")
}

func (s *Suite) TestLockdownError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		Phase: migration.QUIESCE,
	}
	s.guard.lockdownErr = errors.New("squash")
	w, err := migrationminion.New(migrationminion.Config{
		Facade: s.client,
		Guard:  s.guard,
		Agent:  s.agent,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "squash")
	s.stub.CheckCallNames(c, "Watch", "Lockdown")
}

func (s *Suite) TestNONE(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		Phase: migration.NONE,
	}
	w, err := migrationminion.New(migrationminion.Config{
		Facade: s.client,
		Guard:  s.guard,
		Agent:  s.agent,
	})
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "Watch", "Unlock")
}

func (s *Suite) TestSUCCESS(c *gc.C) {
	addrs := []string{"1.1.1.1:1", "9.9.9.9:9"}
	s.client.watcher.changes <- watcher.MigrationStatus{
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   "top secret",
	}
	w, err := migrationminion.New(migrationminion.Config{
		Facade: s.client,
		Guard:  s.guard,
		Agent:  s.agent,
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-s.agent.configChanged:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for config to be changed")
	}
	workertest.CleanKill(c, w)
	c.Assert(s.agent.conf.addrs, gc.DeepEquals, addrs)
	c.Assert(s.agent.conf.caCert, gc.DeepEquals, "top secret")
	s.stub.CheckCallNames(c, "Watch", "Lockdown")
}

func newStubGuard(stub *jujutesting.Stub) *stubGuard {
	return &stubGuard{stub: stub}
}

type stubGuard struct {
	stub        *jujutesting.Stub
	unlockErr   error
	lockdownErr error
}

func (g *stubGuard) Lockdown(fortress.Abort) error {
	g.stub.AddCall("Lockdown")
	return g.lockdownErr
}

func (g *stubGuard) Unlock() error {
	g.stub.AddCall("Unlock")
	return g.unlockErr
}

func newStubMinionClient(stub *jujutesting.Stub) *stubMinionClient {
	return &stubMinionClient{
		stub:    stub,
		watcher: newStubWatcher(),
	}
}

type stubMinionClient struct {
	stub     *jujutesting.Stub
	watcher  *stubWatcher
	watchErr error
}

func (c *stubMinionClient) Watch() (watcher.MigrationStatusWatcher, error) {
	c.stub.MethodCall(c, "Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
}

func newStubWatcher() *stubWatcher {
	return &stubWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: make(chan watcher.MigrationStatus, 1),
	}
}

type stubWatcher struct {
	worker.Worker
	changes chan watcher.MigrationStatus
}

func (w *stubWatcher) Changes() <-chan watcher.MigrationStatus {
	return w.changes
}

func newStubAgent() *stubAgent {
	return &stubAgent{
		configChanged: make(chan bool),
	}
}

type stubAgent struct {
	agent.Agent
	configChanged chan bool
	conf          stubConfig
}

func (ma *stubAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *stubAgent) ChangeConfig(f agent.ConfigMutator) error {
	defer close(ma.configChanged)
	return f(&ma.conf)
}

type stubConfig struct {
	agent.ConfigSetter

	mu     sync.Mutex
	addrs  []string
	caCert string
}

func (mc *stubConfig) setAddresses(addrs ...string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.addrs = append([]string(nil), addrs...)
}

func (mc *stubConfig) SetAPIHostPorts(servers [][]network.HostPort) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.addrs = nil
	for _, hps := range servers {
		for _, hp := range hps {
			mc.addrs = append(mc.addrs, hp.NetAddr())
		}
	}
}

func (mc *stubConfig) SetCACert(cert string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.caCert = cert
}
