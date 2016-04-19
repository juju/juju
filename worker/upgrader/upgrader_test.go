// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/symlink"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	envtesting "github.com/juju/juju/environs/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
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
	upgradeStepsComplete gate.Lock
	initialCheckComplete gate.Lock
}

type AllowedTargetVersionSuite struct{}

var _ = gc.Suite(&UpgraderSuite{})
var _ = gc.Suite(&AllowedTargetVersionSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// s.machine needs to have IsManager() so that it can get the actual
	// current revision to upgrade to.
	s.state, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageModel)
	// Capture the value of RetryAfter, and use that captured
	// value in the cleanup lambda.
	oldRetryAfter := *upgrader.RetryAfter
	s.AddCleanup(func(*gc.C) {
		*upgrader.RetryAfter = oldRetryAfter
	})
	s.upgradeStepsComplete = gate.NewLock()
	s.initialCheckComplete = gate.NewLock()
}

func (s *UpgraderSuite) patchVersion(v version.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&series.HostSeries, func() string { return v.Series })
	s.PatchValue(&jujuversion.Current, v.Number)
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
	w, err := upgrader.NewAgentUpgrader(
		s.state.Upgrader(),
		agentConfig(s.machine.Tag(), s.DataDir()),
		s.confVersion,
		s.upgradeStepsComplete,
		s.initialCheckComplete,
	)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *UpgraderSuite) TestUpgraderSetsTools(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-precise-amd64")
	err := statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, jc.ErrorIsNil)
	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), vers)
	s.patchVersion(agentTools.Version)
	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	u := s.makeUpgrader(c)
	statetesting.AssertStop(c, u)
	s.expectInitialUpgradeCheckDone(c)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	envtesting.CheckTools(c, gotTools, agentTools)
}

/*
func (s *UpgraderSuite) TestUpgraderSetVersion(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-precise-amd64")
	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	agentTools := envtesting.PrimeTools(c, stor, vers)
	s.patchVersion(agentTools.Version)
	err := os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	statetesting.AssertStop(c, u)
	s.expectInitialUpgradeCheckDone(c)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotTools, gc.DeepEquals, &coretools.Tools{Version: vers})
}
*/

func (s *UpgraderSuite) expectInitialUpgradeCheckDone(c *gc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), jc.IsTrue)
}

func (s *UpgraderSuite) expectInitialUpgradeCheckNotDone(c *gc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), jc.IsFalse)
}

func (s *UpgraderSuite) TestUpgraderUpgradesImmediately(c *gc.C) {
	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	oldVers := version.MustParseBinary("5.4.3-precise-amd64")
	oldTools := envtesting.PrimeTools(c, stor, s.DataDir(), oldVers)
	c.Assert(err, jc.ErrorIsNil)
	s.patchVersion(oldTools.Version)

	newVersion := version.MustParseBinary("5.4.5-precise-amd64")
	newTools := envtesting.AssertUploadFakeToolsVersions(c, stor, newVersion)[0]

	err = statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectInitialUpgradeCheckNotDone(c)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), newTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	newTools.URL = fmt.Sprintf("https://%s/model/%s/tools/5.4.5-precise-amd64",
		s.APIState.Addr(), coretesting.ModelTag.Id())
	envtesting.CheckTools(c, foundTools, newTools)
	c.Assert(foundTools.SHA256, gc.Equals, newTools.SHA256)
}

/*
func (s *UpgraderSuite) TestUpgraderRetryAndChanged(c *gc.C) {
	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	oldTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.patchVersion(oldTools.Version)
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor,  version.MustParseBinary("5.4.5-precise-amd64"))[0]
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
	s.expectInitialUpgradeCheckNotDone(c)

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
*/

func (s *UpgraderSuite) TestChangeAgentTools(c *gc.C) {
	oldTools := &coretools.Tools{
		Version: version.MustParseBinary("1.2.3-quantal-amd64"),
	}
	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	newToolsBinary := "5.4.3-precise-amd64"
	newTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary(newToolsBinary))
	s.patchVersion(newTools.Version)
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
	s.patchVersion(oldVersion)

	newVersion := version.MustParseBinary("5.4.3-quantal-amd64")
	err := statetesting.SetAgentVersion(s.State, newVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	// Install tools matching the new version in the data directory
	// but *not* in environment storage. The upgrader should find the
	// downloaded tools without looking in environment storage.
	envtesting.InstallFakeDownloadedTools(c, s.DataDir(), newVersion)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  oldVersion,
		NewTools:  newVersion,
		DataDir:   s.DataDir(),
	})
}

func (s *UpgraderSuite) TestUpgraderRefusesToDowngradeMinorVersions(c *gc.C) {
	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.patchVersion(origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, version.MustParseBinary("5.3.3-precise-amd64"))[0]
	err = statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectInitialUpgradeCheckDone(c)
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
	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.patchVersion(origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, version.MustParseBinary("5.4.2-precise-amd64"))[0]
	err = statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectInitialUpgradeCheckNotDone(c)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  downgradeTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	downgradeTools.URL = fmt.Sprintf("https://%s/model/%s/tools/5.4.2-precise-amd64",
		s.APIState.Addr(), coretesting.ModelTag.Id())
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradeToOrigVersionIfUpgradeInProgress(c *gc.C) {
	// note: otherwise illegal version jump
	downgradeVersion := version.MustParseBinary("5.3.0-precise-amd64")
	s.confVersion = downgradeVersion.Number

	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.patchVersion(origTools.Version)
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, downgradeVersion)[0]
	err = statetesting.SetAgentVersion(s.State, downgradeVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectInitialUpgradeCheckNotDone(c)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  downgradeVersion,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	downgradeTools.URL = fmt.Sprintf("https://%s/model/%s/tools/5.3.0-precise-amd64",
		s.APIState.Addr(), coretesting.ModelTag.Id())
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderRefusesDowngradeToOrigVersionIfUpgradeNotInProgress(c *gc.C) {
	downgradeVersion := version.MustParseBinary("5.3.0-precise-amd64")
	s.confVersion = downgradeVersion.Number
	s.upgradeStepsComplete.Unlock()

	stor, err := s.State.ToolsStorage()
	defer stor.Close()

	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), version.MustParseBinary("5.4.3-precise-amd64"))
	s.patchVersion(origTools.Version)
	envtesting.AssertUploadFakeToolsVersions(
		c, stor, downgradeVersion)
	err = statetesting.SetAgentVersion(s.State, downgradeVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = u.Stop()
	s.expectInitialUpgradeCheckDone(c)

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
