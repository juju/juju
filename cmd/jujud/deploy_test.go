// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"reflect"
	"sort"
	"sync"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	apideployer "launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/worker/deployer"
)

// fakeManager allows us to test deployments without actually deploying units
// to the local system. It's slightly uncomfortably complex because it needs
// to use the *state.State opened within the agent's runOnce -- not the one
// created in the test -- to StartSync and cause the task to actually start
// a sync and observe changes to the set of desired units (and thereby run
// deployment tests in a reasonable amount of time).
type fakeContext struct {
	mu       sync.Mutex
	deployed set.Strings
	st       *state.State
	inited   chan struct{}
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

func (ctx *fakeContext) waitDeployed(c *C, want ...string) {
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
				c.Assert(err, IsNil)
				c.Fatalf("unexpected units: %#v", got)
			case <-time.After(testing.ShortWait):
				got, err := ctx.DeployedUnits()
				c.Assert(err, IsNil)
				if reflect.DeepEqual(got, want) {
					return
				}
			}
		}
	}
}

func patchDeployContext(c *C, st *state.State, expectInfo *state.Info, expectDataDir string) (*fakeContext, func()) {
	ctx := &fakeContext{
		inited: make(chan struct{}),
	}
	e0 := *expectInfo
	expectInfo = &e0
	orig := newDeployContext
	newDeployContext = func(dst *apideployer.State, dataDir string) (deployer.Context, error) {
		caCert, err := dst.CACert()
		if err != nil {
			return nil, err
		}
		stateAddrs, err := dst.StateAddresses()
		if err != nil {
			return nil, err
		}
		c.Check(stateAddrs, DeepEquals, expectInfo.Addrs)
		c.Check(caCert, DeepEquals, expectInfo.CACert)
		c.Check(dataDir, Equals, expectDataDir)
		ctx.st = st
		close(ctx.inited)
		return ctx, nil
	}
	return ctx, func() { newDeployContext = orig }
}
