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
	st       *state.State
	inited   chan struct{}
}

func (mgr *fakeManager) DeployUnit(unitName, _ string) error {
	mgr.mu.Lock()
	mgr.deployed[unitName] = true
	mgr.mu.Unlock()
	return nil
}

func (mgr *fakeManager) RecallUnit(unitName string) error {
	mgr.mu.Lock()
	delete(mgr.deployed, unitName)
	mgr.mu.Unlock()
	return nil
}

func (mgr *fakeManager) DeployedUnits() ([]string, error) {
	var unitNames []string
	mgr.mu.Lock()
	for unitName := range mgr.deployed {
		unitNames = append(unitNames, unitName)
	}
	mgr.mu.Unlock()
	sort.Strings(unitNames)
	return unitNames, nil
}

func (mgr *fakeManager) waitDeployed(c *C, want ...string) {
	sort.Strings(want)
	timeout := time.After(500 * time.Millisecond)
	select {
	case <-timeout:
		c.Fatalf("manager never initialized")
	case <-mgr.inited:
		for {
			mgr.st.StartSync()
			select {
			case <-timeout:
				got, err := mgr.DeployedUnits()
				c.Assert(err, IsNil)
				c.Fatalf("unexpected units: %#v", got)
			case <-time.After(50 * time.Millisecond):
				got, err := mgr.DeployedUnits()
				c.Assert(err, IsNil)
				if reflect.DeepEqual(got, want) {
					return
				}
			}
		}
	}
	panic("unreachable")
}

func patchDeployManager(c *C, expectInfo *state.Info, expectDataDir string) (*fakeManager, func()) {
	mgr := &fakeManager{
		deployed: map[string]bool{},
		inited:   make(chan struct{}),
	}
	orig := newDeployManager
	newDeployManager = func(st *state.State, info *state.Info, dataDir string) deployer.Manager {
		c.Check(info.Addrs, DeepEquals, expectInfo.Addrs)
		c.Check(info.CACert, DeepEquals, expectInfo.CACert)
		c.Check(info.EntityName, Equals, expectInfo.EntityName)
		c.Check(info.Password, Equals, "")
		c.Check(dataDir, Equals, expectDataDir)
		mgr.st = st
		close(mgr.inited)
		return mgr
	}
	return mgr, func() { newDeployManager = orig }
}
