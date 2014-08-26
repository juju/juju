// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/upgrader"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type UpgraderSuite struct {
	jujutesting.JujuConnSuite

	machine       *state.Machine
	state         *api.State
	oldRetryAfter func() <-chan time.Time
}

type AllowedTargetVersionSuite struct{}

var _ = gc.Suite(&UpgraderSuite{})
var _ = gc.Suite(&AllowedTargetVersionSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// s.machine needs to have IsManager() so that it can get the actual
	// current revision to upgrade to.
	s.state, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	// Capture the value of RetryAfter, and use that captured
	// value in the cleanup lambda.
	oldRetryAfter := *upgrader.RetryAfter
	s.AddCleanup(func(*gc.C) {
		*upgrader.RetryAfter = oldRetryAfter
	})
}

type mockConfig struct {
	agent.Config
	tag     string
	datadir string
}

func (mock *mockConfig) Tag() string {
	return mock.tag
}

func (mock *mockConfig) DataDir() string {
	return mock.datadir
}

func agentConfig(tag, datadir string) agent.Config {
	return &mockConfig{tag: tag, datadir: datadir}
}

func (s *UpgraderSuite) makeUpgrader() *upgrader.Upgrader {
	config := agentConfig(s.machine.Tag().String(), s.DataDir())
	return upgrader.NewUpgrader(s.state.Upgrader(), config)
}

func (s *UpgraderSuite) TestUpgraderSetsTools(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-precise-amd64")
	err := statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, gc.IsNil)
	stor := s.Conn.Environ.Storage()
	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), vers)
	s.PatchValue(&version.Current, agentTools.Version)
	err = envtools.MergeAndWriteMetadata(stor, coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	u := s.makeUpgrader()
	statetesting.AssertStop(c, u)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, gc.IsNil)
	envtesting.CheckTools(c, gotTools, agentTools)
}

func (s *UpgraderSuite) TestUpgraderSetVersion(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-precise-amd64")
	agentTools := envtesting.PrimeTools(c, s.Conn.Environ.Storage(), s.DataDir(), vers)
	s.PatchValue(&version.Current, agentTools.Version)
	err := os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, gc.IsNil)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, gc.IsNil)

	u := s.makeUpgrader()
	statetesting.AssertStop(c, u)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(gotTools, gc.DeepEquals, &coretools.Tools{Version: version.Current})
}

func (s *UpgraderSuite) TestUpgraderUpgradesImmediately(c *gc.C) {
	stor := s.Conn.Environ.Storage()
	oldTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, oldTools.Version)
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, version.MustParseBinary("5.4.5-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, gc.IsNil)

	// Make the download take a while so that we verify that
	// the download happens before the upgrader checks if
	// it's been stopped.
	dummy.SetStorageDelay(coretesting.ShortWait)

	u := s.makeUpgrader()
	err = u.Stop()
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), newTools.Version)
	c.Assert(err, gc.IsNil)
	envtesting.CheckTools(c, foundTools, newTools)
}

func (s *UpgraderSuite) TestUpgraderRetryAndChanged(c *gc.C) {
	stor := s.Conn.Environ.Storage()
	oldTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, oldTools.Version)
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, version.MustParseBinary("5.4.5-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, gc.IsNil)

	retryc := make(chan time.Time)
	*upgrader.RetryAfter = func() <-chan time.Time {
		c.Logf("replacement retry after")
		return retryc
	}
	dummy.Poison(s.Conn.Environ.Storage(), envtools.StorageName(newTools.Version), fmt.Errorf("a non-fatal dose"))
	u := s.makeUpgrader()
	defer u.Stop()

	for i := 0; i < 3; i++ {
		select {
		case retryc <- time.Now():
		case <-time.After(coretesting.LongWait):
			c.Fatalf("upgrader did not retry (attempt %d)", i)
		}
	}

	// Make it upgrade to some newer tools that can be
	// downloaded ok; it should stop retrying, download
	// the newer tools and exit.
	newerTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.Conn.Environ.Storage(), version.MustParseBinary("5.4.6-precise-amd64"))[0]

	err = statetesting.SetAgentVersion(s.State, newerTools.Version.Number)
	c.Assert(err, gc.IsNil)

	s.BackingState.StartSync()
	done := make(chan error)
	go func() {
		done <- u.Wait()
	}()
	select {
	case err := <-done:
		envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
			AgentName: s.machine.Tag().String(),
			OldTools:  oldTools.Version,
			NewTools:  newerTools.Version,
			DataDir:   s.DataDir(),
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("upgrader did not quit after upgrading")
	}
}

func (s *UpgraderSuite) TestChangeAgentTools(c *gc.C) {
	oldTools := &coretools.Tools{
		Version: version.MustParseBinary("1.2.3-quantal-amd64"),
	}
	stor := s.Conn.Environ.Storage()
	newTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, newTools.Version)
	err := envtools.MergeAndWriteMetadata(stor, coretools.List{newTools}, envtools.DoNotWriteMirrors)
	c.Assert(err, gc.IsNil)
	ugErr := &upgrader.UpgradeReadyError{
		AgentName: "anAgent",
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	}
	err = ugErr.ChangeAgentTools()
	c.Assert(err, gc.IsNil)
	link, err := os.Readlink(agenttools.ToolsDir(s.DataDir(), "anAgent"))
	c.Assert(err, gc.IsNil)
	c.Assert(link, gc.Equals, newTools.Version.String())
}

func (s *UpgraderSuite) TestEnsureToolsChecksBeforeDownloading(c *gc.C) {
	stor := s.Conn.Environ.Storage()
	newTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, newTools.Version)
	// We've already downloaded the tools, so change the URL to be
	// something invalid and ensure we don't actually get an error, because
	// it doesn't actually do an HTTP request
	u := s.makeUpgrader()
	newTools.URL = "http://0.1.2.3/invalid/path/tools.tgz"
	err := upgrader.EnsureTools(u, newTools, utils.VerifySSLHostnames)
	c.Assert(err, gc.IsNil)
}

func (s *UpgraderSuite) TestUpgraderRefusesToDowngradeMinorVersions(c *gc.C) {
	stor := s.Conn.Environ.Storage()
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, version.MustParseBinary("5.3.3-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, gc.IsNil)

	u := s.makeUpgrader()
	err = u.Stop()
	// If the upgrade would have triggered, we would have gotten an
	// UpgradeReadyError, since it was skipped, we get no error
	c.Check(err, gc.IsNil)
	_, err = agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	// TODO: ReadTools *should* be returning some form of errors.NotFound,
	// however, it just passes back a fmt.Errorf so we live with it
	// c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, "cannot read tools metadata in tools directory.*no such file or directory")
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradingPatchVersions(c *gc.C) {
	stor := s.Conn.Environ.Storage()
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, version.MustParseBinary("5.4.2-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, gc.IsNil)

	dummy.SetStorageDelay(coretesting.ShortWait)

	u := s.makeUpgrader()
	err = u.Stop()
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  downgradeTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	c.Assert(err, gc.IsNil)
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

type allowedTest struct {
	current string
	target  string
	allowed bool
}

func (s *AllowedTargetVersionSuite) TestAllowedTargetVersionSuite(c *gc.C) {
	cases := []allowedTest{
		{current: "1.2.3", target: "1.3.3", allowed: true},
		{current: "1.2.3", target: "1.2.3", allowed: true},
		{current: "1.2.3", target: "2.2.3", allowed: true},
		{current: "1.2.3", target: "1.1.3", allowed: false},
		{current: "1.2.3", target: "1.2.2", allowed: true},
		{current: "1.2.3", target: "0.2.3", allowed: false},
	}
	for i, test := range cases {
		c.Logf("test case %d, %#v", i, test)
		current := version.MustParse(test.current)
		target := version.MustParse(test.target)
		c.Check(upgrader.AllowedTargetVersion(current, target), gc.Equals, test.allowed)
	}
}
