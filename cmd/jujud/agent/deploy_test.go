// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"reflect"
	"sort"
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/testing"
)

// fakeManager allows us to test deployments without actually deploying units
// to the local system.
type fakeContext struct {
	mu          sync.Mutex
	deployed    set.Strings
	agentConfig agent.Config
	inited      *signal
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
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("manager never initialized")
	case <-ctx.inited.triggered():
		timeout := time.After(testing.LongWait)
		for {
			select {
			case <-timeout:
				got, err := ctx.DeployedUnits()
				c.Assert(err, jc.ErrorIsNil)
				c.Fatalf("unexpected units: %#v", got)
			case <-time.After(testing.ShortWait):
				got, err := ctx.DeployedUnits()
				c.Assert(err, jc.ErrorIsNil)
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
