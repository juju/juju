// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"time"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type syncToolsSuite struct {
	testbase.LoggingSuite
	home         *coretesting.FakeHome
	configStore  configstore.Storage
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
	s.configStore, err = configstore.Default()
	c.Assert(err, gc.IsNil)
	s.origSyncTools = syncTools
}

func (s *syncToolsSuite) TearDownTest(c *gc.C) {
	syncTools = s.origSyncTools
	dummy.Reset()
	s.home.Restore()
	s.LoggingSuite.TearDownTest(c)
}

func (s *syncToolsSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
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

var syncToolsCommandTests = []struct {
	description string
	args        []string
	sctx        *sync.SyncContext
}{
	{
		description: "environment as only argument",
		args:        []string{"-e", "test-target"},
		sctx:        &sync.SyncContext{},
	},
	{
		description: "specifying also the synchronization source",
		args:        []string{"-e", "test-target", "--source", "/foo/bar"},
		sctx: &sync.SyncContext{
			Source: "/foo/bar",
		},
	},
	{
		description: "synchronize all version including development",
		args:        []string{"-e", "test-target", "--all", "--dev"},
		sctx: &sync.SyncContext{
			AllVersions: true,
			Dev:         true,
		},
	},
	{
		description: "just make a dry run",
		args:        []string{"-e", "test-target", "--dry-run"},
		sctx: &sync.SyncContext{
			DryRun: true,
		},
	},
	{
		description: "specific public",
		args:        []string{"-e", "test-target", "--public"},
		sctx: &sync.SyncContext{
			Public: true,
		},
	},
	{
		description: "specify version",
		args:        []string{"-e", "test-target", "--version", "1.2"},
		sctx: &sync.SyncContext{
			MajorVersion: 1,
			MinorVersion: 2,
		},
	},
}

func (s *syncToolsSuite) TestSyncToolsCommand(c *gc.C) {
	for i, test := range syncToolsCommandTests {
		c.Logf("test %d: %s", i, test.description)
		targetEnv, err := environs.PrepareFromName("test-target", nullContext(), s.configStore)
		c.Assert(err, gc.IsNil)
		called := false
		syncTools = func(sctx *sync.SyncContext) error {
			c.Assert(sctx.AllVersions, gc.Equals, test.sctx.AllVersions)
			c.Assert(sctx.MajorVersion, gc.Equals, test.sctx.MajorVersion)
			c.Assert(sctx.MinorVersion, gc.Equals, test.sctx.MinorVersion)
			c.Assert(sctx.DryRun, gc.Equals, test.sctx.DryRun)
			c.Assert(sctx.Dev, gc.Equals, test.sctx.Dev)
			c.Assert(sctx.Public, gc.Equals, test.sctx.Public)
			c.Assert(sctx.Source, gc.Equals, test.sctx.Source)
			c.Assert(dummy.IsSameStorage(sctx.Target, targetEnv.Storage()), jc.IsTrue)
			called = true
			return nil
		}
		ctx, err := runSyncToolsCommand(c, test.args...)
		c.Assert(err, gc.IsNil)
		c.Assert(ctx, gc.NotNil)
		c.Assert(called, jc.IsTrue)
		s.Reset(c)
	}
}

func (s *syncToolsSuite) TestSyncToolsCommandTargetDirectory(c *gc.C) {
	called := false
	dir := c.MkDir()
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.AllVersions, gc.Equals, false)
		c.Assert(sctx.DryRun, gc.Equals, false)
		c.Assert(sctx.Dev, gc.Equals, false)
		c.Assert(sctx.Source, gc.Equals, "")
		url, err := sctx.Target.URL("")
		c.Assert(err, gc.IsNil)
		c.Assert(url, gc.Equals, "file://"+dir)
		called = true
		return nil
	}
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--local-dir", dir)
	c.Assert(err, gc.IsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
	s.Reset(c)
}

func (s *syncToolsSuite) TestSyncToolsCommandDeprecatedDestination(c *gc.C) {
	called := false
	dir := c.MkDir()
	syncTools = func(sctx *sync.SyncContext) error {
		c.Assert(sctx.AllVersions, gc.Equals, false)
		c.Assert(sctx.DryRun, gc.Equals, false)
		c.Assert(sctx.Dev, gc.Equals, false)
		c.Assert(sctx.Source, gc.Equals, "")
		url, err := sctx.Target.URL("")
		c.Assert(err, gc.IsNil)
		c.Assert(url, gc.Equals, "file://"+dir)
		called = true
		return nil
	}
	// Register writer.
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("deprecated-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("deprecated-tester")
	// Add deprecated message to be checked.
	messages := []jc.SimpleMessage{
		{loggo.WARNING, "Use of the --destination flag is deprecated in 1.18. Please use --local-dir instead."},
	}
	// Run sync-tools command with --destination flag.
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--destination", dir)
	c.Assert(err, gc.IsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(called, jc.IsTrue)
	// Check deprecated message was logged.
	c.Check(tw.Log, jc.LogMatches, messages)
	s.Reset(c)
}
