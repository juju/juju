// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"reflect"
	"sort"
	"sync"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/set"
)

// fakeManager allows us to test deployments without actually deploying units
// to the local system. It's slightly uncomfortably complex because it needs
// to use the *state.State opened within the agent's runOnce -- not the one
// created in the test -- to StartSync and cause the task to actually start
// a sync and observe changes to the set of desired units (and thereby run
// deployment tests in a reasonable amount of time).
type fakeContext struct {
	mu          sync.Mutex
	deployed    set.Strings
	st          *state.State
	agentConfig agent.Config
	inited      chan struct{}
}

func (ctx *fakeContext) DeployUnit(unitName, _ string) error {
	ctx.mu.Lock()
	ctx.deployed.Add(unitName)
	ctx.mu.Unlock()
	return nil
}

func (ctx *fakeContext) RecallUnit(unitName string) error {
	ctx.mu.Lock()
	ctx.deployed.Remove(unitName)
	ctx.mu.Unlock()
	return nil
}

func (ctx *fakeContext) DeployedUnits() ([]string, error) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.deployed.IsEmpty() {
		return nil, nil
	}
	return ctx.deployed.SortedValues(), nil
}

func (ctx *fakeContext) waitDeployed(c *gc.C, want ...string) {
	sort.Strings(want)
	timeout := time.After(testing.LongWait)
	select {
	case <-timeout:
		c.Fatalf("manager never initialized")
	case <-ctx.inited:
		for {
			ctx.st.StartSync()
			select {
			case <-timeout:
				got, err := ctx.DeployedUnits()
				c.Assert(err, gc.IsNil)
				c.Fatalf("unexpected units: %#v", got)
			case <-time.After(testing.ShortWait):
				got, err := ctx.DeployedUnits()
				c.Assert(err, gc.IsNil)
				if reflect.DeepEqual(got, want) {
					return
				}
			}
		}
	}
}

func (ctx *fakeContext) AgentConfig() agent.Config {
	return ctx.agentConfig
}
