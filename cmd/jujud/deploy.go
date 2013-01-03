package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/deployer"
	"reflect"
	"sort"
	"sync"
	"time"
)

// fakeManager allows us to test deployments without actually deploying units
// to the local system. It's slightly uncomfortably complex because it needs
// to use the *state.State opened within the agent's runOnce -- not the one
// created in the test -- to StartSync and cause the task to actually start
// a sync and observe changes to the set of desired units (and thereby run
// deployment tests in a reasonable amount of time).
type fakeManager struct {
	mu       sync.Mutex
	deployed map[string]bool
	Init     func(*state.State, *state.Info, string)
	st       *state.State
}

func (m *fakeManager) DeployUnit(unitName, _ string) error {
	m.mu.Lock()
	m.deployed[unitName] = true
	m.mu.Unlock()
	return nil
}

func (m *fakeManager) RecallUnit(unitName string) error {
	m.mu.Lock()
	delete(m.deployed, unitName)
	m.mu.Unlock()
	return nil
}

func (m *fakeManager) DeployedUnits() ([]string, error) {
	var unitNames []string
	m.mu.Lock()
	for unitName := range m.deployed {
		unitNames = append(unitNames, unitName)
	}
	m.mu.Unlock()
	sort.Strings(unitNames)
	return unitNames, nil
}

var fake *fakeManager

func newDeployer(st *state.State, w *state.UnitsWatcher, dataDir string) *deployer.Deployer {
	entityName := w.EntityName()
	info := &state.Info{
		EntityName: entityName,
		Addrs:      st.Addrs(),
		CACert:     st.CACert(),
	}
	var mgr deployer.Manager
	if fake != nil {
		fake.Init(st, info, dataDir)
		mgr = fake
	} else {
		// TODO: pick manager kind based on entity name? (once we have a
		// container manager for prinicpal units, that is; for now, there
		// is no distinction between principal and subordinate deployments)
		mgr = deployer.NewSimpleManager(info, dataDir)
	}
	return deployer.NewDeployer(st, mgr, w)
}

func patchDeployManager(c *C, expectInfo *state.Info, expectDataDir string) {
	fake = &fakeManager{
		deployed: map[string]bool{},
		Init: func(st *state.State, info *state.Info, dataDir string) {
			c.Assert(info.Addrs, DeepEquals, expectInfo.Addrs)
			c.Assert(info.CACert, DeepEquals, expectInfo.CACert)
			c.Assert(info.EntityName, Equals, expectInfo.EntityName)
			c.Assert(info.Password, Equals, "")
			c.Assert(dataDir, Equals, expectDataDir)
			fake.st = st
		},
	}
}

func waitDeployed(c *C, want ...string) {
	sort.Strings(want)
	timeout := time.After(500 * time.Millisecond)
	for {
		fake.st.StartSync()
		select {
		case <-timeout:
			got, err := fake.DeployedUnits()
			c.Assert(err, IsNil)
			c.Fatalf("unexpected units: %#v", got)
		case <-time.After(50 * time.Millisecond):
			got, err := fake.DeployedUnits()
			c.Assert(err, IsNil)
			if reflect.DeepEqual(got, want) {
				return
			}
		}
	}
	panic("unreachable")
}

func resetDeployManager() {
	fake = nil
}
