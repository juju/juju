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
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
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

	machine              *state.Machine
	state                api.Connection
	oldRetryAfter        func() <-chan time.Time
	confVersion          version.Number
	upgradeRunning       bool
	agentUpgradeComplete chan struct{}
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
	s.agentUpgradeComplete = make(chan struct{})
}

type mockConfig struct {
	agent.Config
	tag     names.Tag
	datadir string
	version version.Number
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func (mock *mockConfig) DataDir() string {
	return mock.datadir
}

func agentConfig(tag names.Tag, datadir string) agent.Config {
	return &mockConfig{
		tag:     tag,
		datadir: datadir,
	}
}

func (s *UpgraderSuite) makeUpgrader(c *gc.C) *upgrader.Upgrader {
	return upgrader.NewAgentUpgrader(
		s.state.Upgrader(),
		agentConfig(s.machine.Tag(), s.DataDir()),
		s.confVersion,
		func() bool { return s.upgradeRunning },
		s.agentUpgradeComplete,
	)
}

func (s *UpgraderSuite) TestUpgraderSetsTools(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-precise-amd64")
	err := statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, jc.ErrorIsNil)
	stor := s.DefaultToolsStorage
	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), vers)
	s.PatchValue(&version.Current, agentTools.Version)
	err = envtools.MergeAndWriteMetadata(stor, "released", "released", coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	u := s.makeUpgrader(c)
	statetesting.AssertStop(c, u)
	s.expectUpgradeChannelClosed(c)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	envtesting.CheckTools(c, gotTools, agentTools)
}

func (s *UpgraderSuite) TestUpgraderSetVersion(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-precise-amd64")
	agentTools := envtesting.PrimeTools(c, s.DefaultToolsStorage, s.DataDir(), s.Environ.Config().AgentStream(), vers)
	s.PatchValue(&version.Current, agentTools.Version)
	err := os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	statetesting.AssertStop(c, u)
	s.expectUpgradeChannelClosed(c)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotTools, gc.DeepEquals, &coretools.Tools{Version: version.Current})
}

func (s *UpgraderSuite) expectUpgradeChannelClosed(c *gc.C) {
	select {
	case <-s.agentUpgradeComplete:
	default:
		c.Fail()
	}
}

func (s *UpgraderSuite) expectUpgradeChannelNotClosed(c *gc.C) {
	select {
	case <-s.agentUpgradeComplete:
		c.Fail()
	default:
	}
}

func (s *UpgraderSuite) TestUpgraderUpgradesImmediately(c *gc.C) {
	stor := s.DefaultToolsStorage
	oldTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, oldTools.Version)
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.5-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	// Make the download take a while so that we verify that
	// the download happens before the upgrader checks if
	// it's been stopped.
	dummy.SetStorageDelay(coretesting.ShortWait)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectUpgradeChannelNotClosed(c)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), newTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	newTools.URL = fmt.Sprintf("https://%s/environment/%s/tools/5.4.5-precise-amd64",
		s.APIState.Addr(), coretesting.EnvironmentTag.Id())
	envtesting.CheckTools(c, foundTools, newTools)
}

func (s *UpgraderSuite) TestUpgraderRetryAndChanged(c *gc.C) {
	stor := s.DefaultToolsStorage
	oldTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, oldTools.Version)
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.5-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	retryc := make(chan time.Time)
	*upgrader.RetryAfter = func() <-chan time.Time {
		c.Logf("replacement retry after")
		return retryc
	}
	err = stor.Remove(envtools.StorageName(newTools.Version, "released"))
	c.Assert(err, jc.ErrorIsNil)
	u := s.makeUpgrader(c)
	defer u.Stop()
	s.expectUpgradeChannelNotClosed(c)

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
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.6-precise-amd64"))[0]

	err = statetesting.SetAgentVersion(s.State, newerTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

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
	stor := s.DefaultToolsStorage
	newToolsBinary := "5.4.3-precise-amd64"
	newTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary(newToolsBinary))
	s.PatchValue(&version.Current, newTools.Version)
	err := envtools.MergeAndWriteMetadata(stor, "released", "released", coretools.List{newTools}, envtools.DoNotWriteMirrors)
	c.Assert(err, jc.ErrorIsNil)
	ugErr := &upgrader.UpgradeReadyError{
		AgentName: "anAgent",
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	}
	err = ugErr.ChangeAgentTools()
	c.Assert(err, jc.ErrorIsNil)
	target := agenttools.ToolsDir(s.DataDir(), newToolsBinary)
	link, err := symlink.Read(agenttools.ToolsDir(s.DataDir(), "anAgent"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(link, jc.SamePath, target)
}

func (s *UpgraderSuite) TestUsesAlreadyDownloadedToolsIfAvailable(c *gc.C) {
	oldVersion := version.MustParseBinary("1.2.3-quantal-amd64")
	s.PatchValue(&version.Current, oldVersion)

	newVersion := version.MustParseBinary("5.4.3-quantal-amd64")
	err := statetesting.SetAgentVersion(s.State, newVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	// Install tools matching the new version in the data directory
	// but *not* in environment storage. The upgrader should find the
	// downloaded tools without looking in environment storage.
	envtesting.InstallFakeDownloadedTools(c, s.DataDir(), newVersion)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectUpgradeChannelNotClosed(c)

	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  oldVersion,
		NewTools:  newVersion,
		DataDir:   s.DataDir(),
	})
}

func (s *UpgraderSuite) TestUpgraderRefusesToDowngradeMinorVersions(c *gc.C) {
	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.3.3-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectUpgradeChannelClosed(c)
	// If the upgrade would have triggered, we would have gotten an
	// UpgradeReadyError, since it was skipped, we get no error
	c.Check(err, jc.ErrorIsNil)
	_, err = agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	// TODO: ReadTools *should* be returning some form of errors.NotFound,
	// however, it just passes back a fmt.Errorf so we live with it
	// c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, "cannot read tools metadata in tools directory.*"+utils.NoSuchFileErrRegexp)
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradingPatchVersions(c *gc.C) {
	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.2-precise-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	dummy.SetStorageDelay(coretesting.ShortWait)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectUpgradeChannelNotClosed(c)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  downgradeTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	downgradeTools.URL = fmt.Sprintf("https://%s/environment/%s/tools/5.4.2-precise-amd64",
		s.APIState.Addr(), coretesting.EnvironmentTag.Id())
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradeToOrigVersionIfUpgradeInProgress(c *gc.C) {
	// note: otherwise illegal version jump
	downgradeVersion := version.MustParseBinary("5.3.0-precise-amd64")
	s.confVersion = downgradeVersion.Number
	s.upgradeRunning = true

	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), downgradeVersion)[0]
	err := statetesting.SetAgentVersion(s.State, downgradeVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	dummy.SetStorageDelay(coretesting.ShortWait)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectUpgradeChannelNotClosed(c)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  downgradeVersion,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	downgradeTools.URL = fmt.Sprintf("https://%s/environment/%s/tools/5.3.0-precise-amd64",
		s.APIState.Addr(), coretesting.EnvironmentTag.Id())
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderRefusesDowngradeToOrigVersionIfUpgradeNotInProgress(c *gc.C) {
	downgradeVersion := version.MustParseBinary("5.3.0-precise-amd64")
	s.confVersion = downgradeVersion.Number
	s.upgradeRunning = false

	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.PatchValue(&version.Current, origTools.Version)
	envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), downgradeVersion)
	err := statetesting.SetAgentVersion(s.State, downgradeVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	dummy.SetStorageDelay(coretesting.ShortWait)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectUpgradeChannelClosed(c)

	// If the upgrade would have triggered, we would have gotten an
	// UpgradeReadyError, since it was skipped, we get no error
	c.Check(err, jc.ErrorIsNil)
}

type allowedTest struct {
	original       string
	current        string
	target         string
	upgradeRunning bool
	allowed        bool
}

func (s *AllowedTargetVersionSuite) TestAllowedTargetVersionSuite(c *gc.C) {
	cases := []allowedTest{
		{original: "1.2.3", current: "1.2.3", upgradeRunning: false, target: "1.3.3", allowed: true},
		{original: "1.2.3", current: "1.2.3", upgradeRunning: false, target: "1.2.3", allowed: true},
		{original: "1.2.3", current: "1.2.3", upgradeRunning: false, target: "2.2.3", allowed: true},
		{original: "1.2.3", current: "1.2.3", upgradeRunning: false, target: "1.1.3", allowed: false},
		{original: "1.2.3", current: "1.2.3", upgradeRunning: false, target: "1.2.2", allowed: true}, // downgrade between builds
		{original: "1.2.3", current: "1.2.3", upgradeRunning: false, target: "0.2.3", allowed: false},
		{original: "0.2.3", current: "1.2.3", upgradeRunning: false, target: "0.2.3", allowed: false},
		{original: "0.2.3", current: "1.2.3", upgradeRunning: true, target: "0.2.3", allowed: true}, // downgrade during upgrade
	}
	for i, test := range cases {
		c.Logf("test case %d, %#v", i, test)
		original := version.MustParse(test.original)
		current := version.MustParse(test.current)
		target := version.MustParse(test.target)
		result := upgrader.AllowedTargetVersion(original, current, test.upgradeRunning, target)
		c.Check(result, gc.Equals, test.allowed)
	}
}
