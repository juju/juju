// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"os"
	"path"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/symlink"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	upgraderapi "github.com/juju/juju/api/agent/upgrader"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades"
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
	confVersion          version.Number
	upgradeStepsComplete gate.Lock
	initialCheckComplete gate.Lock
	clock                *testclock.Clock
}

type AllowedTargetVersionSuite struct{}

var _ = gc.Suite(&UpgraderSuite{})
var _ = gc.Suite(&AllowedTargetVersionSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// s.machine needs to have IsManager() so that it can get the actual
	// current revision to upgrade to.
	s.state, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageModel)

	// For expediency we assume that upgrade-steps have run as the default.
	// Create a new locked gate for alternative test composition.
	s.upgradeStepsComplete = gate.NewLock()
	s.upgradeStepsComplete.Unlock()

	s.initialCheckComplete = gate.NewLock()
	s.clock = testclock.NewClock(time.Now())
}

func (s *UpgraderSuite) patchVersion(v version.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })
	vers := v.Number
	vers.Build = 666
	s.PatchValue(&jujuversion.Current, vers)
}

type mockConfig struct {
	agent.Config
	tag     names.Tag
	datadir string
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
	w, err := upgrader.NewAgentUpgrader(upgrader.Config{
		Clock:                       s.clock,
		Logger:                      loggo.GetLogger("test"),
		State:                       upgraderapi.NewState(s.state),
		AgentConfig:                 agentConfig(s.machine.Tag(), s.DataDir()),
		OrigAgentVersion:            s.confVersion,
		UpgradeStepsWaiter:          s.upgradeStepsComplete,
		InitialUpgradeCheckComplete: s.initialCheckComplete,
		CheckDiskSpace:              func(string, uint64) error { return nil },
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *UpgraderSuite) TestUpgraderSetsTools(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	err := statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, jc.ErrorIsNil)

	store := s.DefaultToolsStorage
	agentTools := envtesting.PrimeTools(c, store, s.DataDir(), s.Environ.Config().AgentStream(), vers)
	s.patchVersion(agentTools.Version)

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = envtools.MergeAndWriteMetadata(
		ss, store, "released", "released", coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	u := s.makeUpgrader(c)
	s.waitForUpgradeCheck(c)
	workertest.CleanKill(c, u)

	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	gotTools, err := s.machine.AgentTools()

	c.Assert(err, jc.ErrorIsNil)
	agentTools.Version.Build = 666
	envtesting.CheckTools(c, gotTools, agentTools)
}

func (s *UpgraderSuite) TestUpgraderSetVersion(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	agentTools := envtesting.PrimeTools(c, s.DefaultToolsStorage, s.DataDir(), s.Environ.Config().AgentStream(), vers)
	s.patchVersion(agentTools.Version)
	err := os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	s.waitForUpgradeCheck(c)
	workertest.CleanKill(c, u)

	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	vers.Build = 666
	c.Assert(gotTools, gc.DeepEquals, &coretools.Tools{Version: vers})
}

func (s *UpgraderSuite) TestUpgraderWaitsForUpgradeStepsGate(c *gc.C) {
	// Replace with a locked gate.
	s.upgradeStepsComplete = gate.NewLock()

	stor := s.DefaultToolsStorage

	oldTools := envtesting.PrimeTools(
		c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(oldTools.Version)

	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.5-ubuntu-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	workertest.CheckAlive(c, u)

	s.expectInitialUpgradeCheckNotDone(c)

	// No upgrade-ready error.
	workertest.CleanKill(c, u)
}

func (s *UpgraderSuite) TestUpgraderUpgradesImmediately(c *gc.C) {
	stor := s.DefaultToolsStorage

	oldTools := envtesting.PrimeTools(
		c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(oldTools.Version)

	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.5-ubuntu-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), newTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	url := s.APIState.Addr()
	url.Scheme = "https"
	url.Path = path.Join(url.Path, "model", coretesting.ModelTag.Id(), "tools", "5.4.5-ubuntu-amd64")
	newTools.URL = url.String()
	envtesting.CheckTools(c, foundTools, newTools)
}

func (s *UpgraderSuite) TestUpgraderRetryAndChanged(c *gc.C) {
	stor := s.DefaultToolsStorage

	oldTools := envtesting.PrimeTools(
		c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(oldTools.Version)

	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.5-ubuntu-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	err = stor.Remove(envtools.StorageName(newTools.Version, "released"))
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	defer func() { _ = workertest.CheckKilled(c, u) }()
	s.expectInitialUpgradeCheckNotDone(c)

	for i := 0; i < 3; i++ {
		err := s.clock.WaitAdvance(5*time.Second, coretesting.LongWait, 1)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Make it upgrade to some newer tools that can be
	// downloaded ok; it should stop retrying, download
	// the newer tools and exit.
	newerTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.6-ubuntu-amd64"))[0]

	err = statetesting.SetAgentVersion(s.State, newerTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	done := make(chan error)
	go func() {
		done <- u.Wait()
	}()
	select {
	case err := <-done:
		envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
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
	oldTools := &coretools.Tools{Version: version.MustParseBinary("1.2.3-ubuntu-amd64")}

	store := s.DefaultToolsStorage
	newToolsBinary := "5.4.3-ubuntu-amd64"
	newTools := envtesting.PrimeTools(
		c, store, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary(newToolsBinary))
	s.patchVersion(newTools.Version)

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err := envtools.MergeAndWriteMetadata(
		ss, store, "released", "released", coretools.List{newTools}, envtools.DoNotWriteMirrors)
	c.Assert(err, jc.ErrorIsNil)

	ugErr := &agenterrors.UpgradeReadyError{
		AgentName: "anAgent",
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	}
	err = ugErr.ChangeAgentTools(loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	target := agenttools.ToolsDir(s.DataDir(), newToolsBinary)
	link, err := symlink.Read(agenttools.ToolsDir(s.DataDir(), "anAgent"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(link, jc.SamePath, target)
}

func (s *UpgraderSuite) TestUsesAlreadyDownloadedToolsIfAvailable(c *gc.C) {
	oldVersion := version.MustParseBinary("1.2.3-ubuntu-amd64")
	s.patchVersion(oldVersion)

	newVersion := version.MustParseBinary("5.4.3-ubuntu-amd64")
	err := statetesting.SetAgentVersion(s.State, newVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	// Install tools matching the new version in the data directory
	// but *not* in environment storage. The upgrader should find the
	// downloaded tools without looking in environment storage.
	envtesting.InstallFakeDownloadedTools(c, s.DataDir(), newVersion)

	u := s.makeUpgrader(c)
	err = workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  oldVersion,
		NewTools:  newVersion,
		DataDir:   s.DataDir(),
	})
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradingMinorVersions(c *gc.C) {
	// We allow this scenario to allow reverting upgrades by restoring
	// a backup from the previous version.
	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(
		c, stor, s.DataDir(), s.Environ.Config().AgentStream(), version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(origTools.Version)

	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.3.3-ubuntu-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  downgradeTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	url := s.APIState.Addr()
	url.Scheme = "https"
	url.Path = path.Join(url.Path, "model", coretesting.ModelTag.Id(), "tools", "5.3.3-ubuntu-amd64")
	downgradeTools.URL = url.String()
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderForbidsDowngradingToMajorVersion(c *gc.C) {
	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("2.4.3-ubuntu-amd64"))
	s.patchVersion(origTools.Version)

	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("1.25.3-ubuntu-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	s.waitForUpgradeCheck(c)
	err = worker.Stop(u)

	// If the upgrade had been allowed we would get an UpgradeReadyError.
	c.Assert(err, jc.ErrorIsNil)
	_, err = agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	// TODO: ReadTools *should* be returning some form of
	// errors.NotFound, however, it just passes back a fmt.Errorf so
	// we live with it c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, "cannot read agent metadata in directory.*"+utils.NoSuchFileErrRegexp)
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradingPatchVersions(c *gc.C) {
	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(origTools.Version)

	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.2-ubuntu-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, downgradeTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  downgradeTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	url := s.APIState.Addr()
	url.Scheme = "https"
	url.Path = path.Join(url.Path, "model", coretesting.ModelTag.Id(), "tools", "5.4.2-ubuntu-amd64")
	downgradeTools.URL = url.String()
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradeToPriorMinorVersion(c *gc.C) {
	// We now allow this to support restoring
	// a backup from a previous version.
	downgradeVersion := version.MustParseBinary("5.3.0-ubuntu-amd64")
	s.confVersion = downgradeVersion.Number

	stor := s.DefaultToolsStorage
	origTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(origTools.Version)

	envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), downgradeVersion)

	prevTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), downgradeVersion)[0]

	err := statetesting.SetAgentVersion(s.State, downgradeVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	u := s.makeUpgrader(c)
	err = workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: s.machine.Tag().String(),
		OldTools:  origTools.Version,
		NewTools:  prevTools.Version,
		DataDir:   s.DataDir(),
	})
	foundTools, err := agenttools.ReadTools(s.DataDir(), prevTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	url := s.APIState.Addr()
	url.Scheme = "https"
	url.Path = path.Join(url.Path, "model", coretesting.ModelTag.Id(), "tools", "5.3.0-ubuntu-amd64")
	prevTools.URL = url.String()
	envtesting.CheckTools(c, foundTools, prevTools)
}

func (s *UpgraderSuite) TestChecksSpaceBeforeDownloading(c *gc.C) {
	stor := s.DefaultToolsStorage
	oldTools := envtesting.PrimeTools(c, stor, s.DataDir(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(oldTools.Version)

	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, stor, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(),
		version.MustParseBinary("5.4.5-ubuntu-amd64"))[0]
	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, jc.ErrorIsNil)

	// We want to wait for the model to settle so that we get a single event
	// from the version watcher.
	// If we start the worker too quickly after setting the new tools,
	// it is possible to get 2 watcher changes - the guaranteed initial event
	// and *then* the one for the change.
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	var diskSpaceStub testing.Stub
	diskSpaceStub.SetErrors(nil, errors.Errorf("full-up"))
	diskSpaceChecked := make(chan struct{}, 1)

	u, err := upgrader.NewAgentUpgrader(upgrader.Config{
		Clock:                       s.clock,
		Logger:                      loggo.GetLogger("test"),
		State:                       upgraderapi.NewState(s.state),
		AgentConfig:                 agentConfig(s.machine.Tag(), s.DataDir()),
		OrigAgentVersion:            s.confVersion,
		UpgradeStepsWaiter:          s.upgradeStepsComplete,
		InitialUpgradeCheckComplete: s.initialCheckComplete,
		CheckDiskSpace: func(dir string, size uint64) error {
			diskSpaceStub.AddCall("CheckDiskSpace", dir, size)

			// CheckDiskSpace is called twice in checkForSpace.
			// We only care that we arrived there, so if we've already buffered
			// a write, just proceed.
			select {
			case diskSpaceChecked <- struct{}{}:
			default:
			}

			return diskSpaceStub.NextErr()
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-diskSpaceChecked:
		workertest.CleanKill(c, u)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for disk space check.")
	}

	s.expectInitialUpgradeCheckNotDone(c)

	c.Assert(diskSpaceStub.Calls(), gc.HasLen, 2)
	diskSpaceStub.CheckCall(c, 0, "CheckDiskSpace", s.DataDir(), upgrades.MinDiskSpaceMib)
	diskSpaceStub.CheckCall(c, 1, "CheckDiskSpace", os.TempDir(), upgrades.MinDiskSpaceMib)

	_, err = agenttools.ReadTools(s.DataDir(), newTools.Version)
	c.Assert(err, gc.ErrorMatches, `cannot read agent metadata in directory.*: no such file or directory`)
}

func (s *UpgraderSuite) waitForUpgradeCheck(c *gc.C) {
	select {
	case <-s.initialCheckComplete.Unlocked():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for initial upgrade check")
	}
}

func (s *UpgraderSuite) expectInitialUpgradeCheckNotDone(c *gc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), jc.IsFalse)
}

type allowedTest struct {
	current string
	target  string
	allowed bool
}

func (s *AllowedTargetVersionSuite) TestAllowedTargetVersionSuite(c *gc.C) {
	cases := []allowedTest{
		{current: "2.7.4", target: "2.8.0", allowed: true},  // normal upgrade
		{current: "2.8.0", target: "2.7.4", allowed: true},  // downgrade caused by restore after upgrade
		{current: "3.8.0", target: "1.2.3", allowed: false}, // can't downgrade to major version 1.x
		{current: "2.7.4", target: "2.7.5", allowed: true},  // point release
		{current: "2.8.0", target: "2.7.4", allowed: true},  // downgrade after upgrade but before config file updated
	}
	for i, test := range cases {
		c.Logf("test case %d, %#v", i, test)
		current := version.MustParse(test.current)
		target := version.MustParse(test.target)
		result := upgrader.AllowedTargetVersion(current, target)
		c.Check(result, gc.Equals, test.allowed)
	}
}
