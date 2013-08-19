// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/provider/dummy"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type syncToolsSuite struct {
	coretesting.LoggingSuite
	home         *coretesting.FakeHome
	targetEnv    environs.Environ
	localStorage string

	origSyncTools func(*sync.SyncContext) error
}

var _ = gc.Suite(&syncToolsSuite{})

func (s *syncToolsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)

	// Create a target environments.yaml and make sure its environment is empty.
	s.home = coretesting.MakeFakeHome(c, `
environments:
    test-target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`)
	var err error
	s.targetEnv, err = environs.NewFromName("test-target")
	c.Assert(err, gc.IsNil)
	envtesting.RemoveAllTools(c, s.targetEnv)
	s.origSyncTools = syncTools
}

func (s *syncToolsSuite) TearDownTest(c *gc.C) {
	syncTools = s.origSyncTools
	dummy.Reset()
	s.home.Restore()
	s.LoggingSuite.TearDownTest(c)
}

func runSyncToolsCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return coretesting.RunCommand(c, &SyncToolsCommand{}, args)
}

func wait(signal chan struct{}) error {
	select {
	case <-signal:
		return nil
	case <-time.After(25 * time.Millisecond):
		return errors.New("timeout")
	}
}

var tests = []struct {
	description string
	args        []string
	sctx        *sync.SyncContext
}{
	{
		description: "environment as only argument",
		args:        []string{"-e", "test-target"},
		sctx: &sync.SyncContext{
			EnvName: "test-target",
		},
	},
	{
		description: "specifying also the synchronization source",
		args:        []string{"-e", "test-target", "--source", "/foo/bar"},
		sctx: &sync.SyncContext{
			EnvName: "test-target",
			Source:  "/foo/bar",
		},
	},
	{
		description: "synchronize all version including development",
		args:        []string{"-e", "test-target", "--all", "--dev"},
		sctx: &sync.SyncContext{
			EnvName:     "test-target",
			AllVersions: true,
			Dev:         true,
		},
	},
	{
		description: "synchronize to public bucket",
		args:        []string{"-e", "test-target", "--public"},
		sctx: &sync.SyncContext{
			EnvName:      "test-target",
			PublicBucket: true,
		},
	},
	{
		description: "just make a dry run",
		args:        []string{"-e", "test-target", "--dry-run"},
		sctx: &sync.SyncContext{
			EnvName: "test-target",
			DryRun:  true,
		},
	},
}

func (s *syncToolsSuite) TestSyncToolsCommand(c *gc.C) {
	for _, test := range tests {
		c.Log(test.description)
		called := make(chan struct{}, 1)
		syncTools = func(sctx *sync.SyncContext) error {
			c.Assert(sctx.EnvName, gc.Equals, test.sctx.EnvName)
			c.Assert(sctx.AllVersions, gc.Equals, test.sctx.AllVersions)
			c.Assert(sctx.DryRun, gc.Equals, test.sctx.DryRun)
			c.Assert(sctx.PublicBucket, gc.Equals, test.sctx.PublicBucket)
			c.Assert(sctx.Dev, gc.Equals, test.sctx.Dev)
			c.Assert(sctx.Source, gc.Equals, test.sctx.Source)
			called <- struct{}{}
			return nil
		}
		ctx, err := runSyncToolsCommand(c, test.args...)
		c.Assert(err, gc.IsNil)
		c.Assert(ctx, gc.NotNil)
		c.Assert(wait(called), gc.IsNil)
	}
}
