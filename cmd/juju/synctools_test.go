// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type syncToolsSuite struct {
	coretesting.LoggingSuite
	home         *coretesting.FakeHome
	targetEnv    environs.Environ
	origVersion  version.Binary
	origLocation string
	storage      *envtesting.EC2HTTPTestStorage
	localStorage string

	origSyncTools func(*sync.SyncContext) error
}

var _ = gc.Suite(&syncToolsSuite{})

func (s *syncToolsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.origVersion = version.Current
	// It's important that this be v1 to match the test data.
	version.Current.Number = version.MustParse("1.2.3")

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
	version.Current = s.origVersion
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
	panic("unreachable")
}

func (s *syncToolsSuite) TestEnvironmentOnly(c *gc.C) {
	called := make(chan struct{}, 1)
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.EnvName, gc.Equals, "test-target")
		called <- struct{}{}
		return nil
	}
	ctx, err := runSyncToolsCommand(c, "-e", "test-target")
	c.Assert(err, gc.IsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(wait(called), gc.IsNil)
}

func (s *syncToolsSuite) TestWithSource(c *gc.C) {
	called := make(chan struct{}, 1)
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.EnvName, gc.Equals, "test-target")
		c.Assert(sctx.Source, gc.Equals, "/foo/bar")
		called <- struct{}{}
		return nil
	}
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--source", "/foo/bar")
	c.Assert(err, gc.IsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(wait(called), gc.IsNil)
}

func (s *syncToolsSuite) TestWithAllAndDev(c *gc.C) {
	called := make(chan struct{}, 1)
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.EnvName, gc.Equals, "test-target")
		c.Assert(sctx.AllVersions, gc.Equals, true)
		c.Assert(sctx.Dev, gc.Equals, true)
		called <- struct{}{}
		return nil
	}
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--all", "--dev")
	c.Assert(err, gc.IsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(wait(called), gc.IsNil)
}

func (s *syncToolsSuite) TestWithPublicAndDry(c *gc.C) {
	called := make(chan struct{}, 1)
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.EnvName, gc.Equals, "test-target")
		c.Assert(sctx.DryRun, gc.Equals, true)
		c.Assert(sctx.PublicBucket, gc.Equals, true)
		called <- struct{}{}
		return nil
	}
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--public", "--dry-run")
	c.Assert(err, gc.IsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(wait(called), gc.IsNil)
}
