// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"os"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/symlink"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/upgrades"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/upgrader"
	"github.com/juju/juju/worker/upgrader/mocks"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/upgrader_mocks.go github.com/juju/juju/worker/upgrader UpgraderClient
func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type UpgraderSuite struct {
	testing.IsolationSuite

	confVersion          version.Number
	upgradeStepsComplete gate.Lock
	initialCheckComplete gate.Lock
	clock                *testclock.Clock

	dataDir string
	store   storage.Storage
}

type AllowedTargetVersionSuite struct{}

var _ = gc.Suite(&UpgraderSuite{})
var _ = gc.Suite(&AllowedTargetVersionSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.dataDir = c.MkDir()
	store, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	s.store = store

	// For expediency we assume that upgrade-steps have run as the default.
	// Create a new locked gate for alternative test composition.
	s.upgradeStepsComplete = gate.NewLock()
	s.upgradeStepsComplete.Unlock()

	s.initialCheckComplete = gate.NewLock()
	s.clock = testclock.NewClock(time.Now())
}

func (s *UpgraderSuite) patchVersion(v version.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })
	s.PatchValue(&jujuversion.Current, v.Number)
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

func (s *UpgraderSuite) makeUpgrader(c *gc.C, client upgrader.UpgraderClient) *upgrader.Upgrader {
	w, err := upgrader.NewAgentUpgrader(upgrader.Config{
		Clock:                       s.clock,
		Logger:                      loggo.GetLogger("test"),
		Client:                      client,
		AgentConfig:                 agentConfig(names.NewMachineTag("666"), s.dataDir),
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
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().DesiredVersion("machine-666").Return(vers.Number, nil)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)

	u := s.makeUpgrader(c, client)
	s.waitForUpgradeCheck(c)
	workertest.CleanKill(c, u)
}

func (s *UpgraderSuite) TestUpgraderSetVersion(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().DesiredVersion("machine-666").Return(vers.Number, nil)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)

	u := s.makeUpgrader(c, client)
	s.waitForUpgradeCheck(c)

	newVersion := vers
	newVersion.Minor++
	client.EXPECT().DesiredVersion("machine-666").Return(newVersion.Number, nil)
	client.EXPECT().Tools("machine-666").Return(coretools.List{}, nil)

	ch <- struct{}{}

	workertest.CleanKill(c, u)
}

func (s *UpgraderSuite) TestUpgraderWaitsForUpgradeStepsGate(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	// Replace with a locked gate.
	s.upgradeStepsComplete = gate.NewLock()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)

	u := s.makeUpgrader(c, client)
	workertest.CheckAlive(c, u)

	s.expectInitialUpgradeCheckNotDone(c)

	// No upgrade-ready error.
	workertest.CleanKill(c, u)
}

func (s *UpgraderSuite) TestUpgraderUpgradesImmediately(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	newVersion := vers
	newVersion.Minor++
	newTools := envtesting.PrimeTools(c, s.store, s.dataDir, "released", newVersion)

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().DesiredVersion("machine-666").Return(newVersion.Number, nil)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)

	u := s.makeUpgrader(c, client)
	err := workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: "machine-666",
		OldTools:  vers,
		NewTools:  newVersion,
		DataDir:   s.dataDir,
	})
	foundTools, err := agenttools.ReadTools(s.dataDir, newVersion)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.CheckTools(c, foundTools, newTools)
}

func (s *UpgraderSuite) TestUpgraderRetryAndChanged(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	newVersion := vers
	newVersion.Minor++

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)

	retryCount := 3

	client.EXPECT().DesiredVersion("machine-666").Return(newVersion.Number, nil).Times(retryCount + 1)
	client.EXPECT().Tools("machine-666").Return(coretools.List{{
		URL: "http://invalid",
	}}, nil).Times(retryCount + 1)

	u := s.makeUpgrader(c, client)
	defer func() { _ = workertest.CheckKilled(c, u) }()
	s.expectInitialUpgradeCheckNotDone(c)

	for i := 0; i < retryCount; i++ {
		err := s.clock.WaitAdvance(5*time.Second, coretesting.LongWait, 1)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Make it upgrade to some newer tools that can be
	// downloaded ok; it should stop retrying, download
	// the newer tools and exit.
	newerVersion := newVersion
	newerVersion.Minor++
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.store, "released", "released",
		newerVersion)[0]

	client.EXPECT().DesiredVersion("machine-666").Return(newerVersion.Number, nil)
	client.EXPECT().Tools("machine-666").Return(coretools.List{newTools}, nil)
	ch <- struct{}{}

	done := make(chan error)
	go func() {
		done <- u.Wait()
	}()
	select {
	case err := <-done:
		envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
			AgentName: "machine-666",
			OldTools:  vers,
			NewTools:  newerVersion,
			DataDir:   s.dataDir,
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("upgrader did not quit after upgrading")
	}
	foundTools, err := agenttools.ReadTools(s.dataDir, newerVersion)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.CheckTools(c, foundTools, newTools)
}

func (s *UpgraderSuite) TestChangeAgentTools(c *gc.C) {
	oldTools := &coretools.Tools{Version: version.MustParseBinary("1.2.3-ubuntu-amd64")}

	newToolsBinary := "5.4.3-ubuntu-amd64"
	newTools := envtesting.PrimeTools(
		c, s.store, s.dataDir, "released", version.MustParseBinary(newToolsBinary))

	ugErr := &agenterrors.UpgradeReadyError{
		AgentName: "anAgent",
		OldTools:  oldTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.dataDir,
	}
	err := ugErr.ChangeAgentTools(loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	target := agenttools.ToolsDir(s.dataDir, newToolsBinary)
	link, err := symlink.Read(agenttools.ToolsDir(s.dataDir, "anAgent"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(link, jc.SamePath, target)
}

func (s *UpgraderSuite) TestUsesAlreadyDownloadedToolsIfAvailable(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)

	newVersion := vers
	newVersion.Minor++

	// Install tools matching the new version in the data directory
	// but *not* in environment storage. The upgrader should find the
	// downloaded tools without looking in environment storage.
	envtesting.InstallFakeDownloadedTools(c, s.dataDir, newVersion)

	client.EXPECT().DesiredVersion("machine-666").Return(newVersion.Number, nil)
	ch <- struct{}{}

	u := s.makeUpgrader(c, client)
	err := workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: "machine-666",
		OldTools:  vers,
		NewTools:  newVersion,
		DataDir:   s.dataDir,
	})
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradingMinorVersions(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	// We allow this scenario to allow reverting upgrades by restoring
	// a backup from the previous version.
	oldVersion := vers
	oldVersion.Minor--
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.store, "released", "released",
		oldVersion)[0]

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)
	client.EXPECT().DesiredVersion("machine-666").Return(oldVersion.Number, nil)
	client.EXPECT().Tools("machine-666").Return(coretools.List{downgradeTools}, nil)

	u := s.makeUpgrader(c, client)
	err := workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: "machine-666",
		OldTools:  vers,
		NewTools:  downgradeTools.Version,
		DataDir:   s.dataDir,
	})
	foundTools, err := agenttools.ReadTools(s.dataDir, downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderForbidsDowngradingToMajorVersion(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	// We allow this scenario to allow reverting upgrades by restoring
	// a backup from the previous version.
	oldVersion := vers
	oldVersion.Major--
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.store, "released", "released",
		oldVersion)[0]

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)
	client.EXPECT().DesiredVersion("machine-666").Return(oldVersion.Number, nil)

	u := s.makeUpgrader(c, client)
	s.waitForUpgradeCheck(c)
	err := worker.Stop(u)

	// If the upgrade had been allowed we would get an UpgradeReadyError.
	c.Assert(err, jc.ErrorIsNil)
	_, err = agenttools.ReadTools(s.dataDir, downgradeTools.Version)
	// TODO: ReadTools *should* be returning some form of
	// errors.NotFound, however, it just passes back a fmt.Errorf so
	// we live with it c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Check(err, gc.ErrorMatches, "cannot read agent metadata in directory.*"+utils.NoSuchFileErrRegexp)
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradingPatchVersions(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-ubuntu-amd64")
	s.patchVersion(vers)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	// We allow this scenario to allow reverting upgrades by restoring
	// a backup from the previous version.
	oldVersion := vers
	oldVersion.Patch--
	downgradeTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.store, "released", "released",
		oldVersion)[0]

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", vers)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)
	client.EXPECT().DesiredVersion("machine-666").Return(oldVersion.Number, nil)
	client.EXPECT().Tools("machine-666").Return(coretools.List{downgradeTools}, nil)

	u := s.makeUpgrader(c, client)
	err := workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: "machine-666",
		OldTools:  vers,
		NewTools:  downgradeTools.Version,
		DataDir:   s.dataDir,
	})
	foundTools, err := agenttools.ReadTools(s.dataDir, downgradeTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.CheckTools(c, foundTools, downgradeTools)
}

func (s *UpgraderSuite) TestUpgraderAllowsDowngradeToPriorMinorVersion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// We now allow this to support restoring
	// a backup from a previous version.
	downgradeVersion := version.MustParseBinary("5.3.0-ubuntu-amd64")
	s.confVersion = downgradeVersion.Number

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	origTools := envtesting.PrimeTools(c, s.store, s.dataDir, "released",
		version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(origTools.Version)

	envtesting.AssertUploadFakeToolsVersions(
		c, s.store, "released", "released", downgradeVersion)

	prevTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.store, "released", "released", downgradeVersion)[0]

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", origTools.Version)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)
	client.EXPECT().DesiredVersion("machine-666").Return(downgradeVersion.Number, nil)
	client.EXPECT().Tools("machine-666").Return(coretools.List{prevTools}, nil)

	u := s.makeUpgrader(c, client)
	err := workertest.CheckKilled(c, u)
	s.expectInitialUpgradeCheckNotDone(c)

	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: "machine-666",
		OldTools:  origTools.Version,
		NewTools:  prevTools.Version,
		DataDir:   s.dataDir,
	})
	foundTools, err := agenttools.ReadTools(s.dataDir, prevTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.CheckTools(c, foundTools, prevTools)
}

func (s *UpgraderSuite) TestChecksSpaceBeforeDownloading(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	oldTools := envtesting.PrimeTools(c, s.store, s.dataDir, "released",
		version.MustParseBinary("5.4.3-ubuntu-amd64"))
	s.patchVersion(oldTools.Version)

	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.store, "released", "released",
		version.MustParseBinary("5.4.5-ubuntu-amd64"))[0]

	client := mocks.NewMockUpgraderClient(ctrl)
	client.EXPECT().SetVersion("machine-666", oldTools.Version)
	client.EXPECT().WatchAPIVersion("machine-666").Return(watch, nil)
	client.EXPECT().DesiredVersion("machine-666").Return(newTools.Version.Number, nil)
	client.EXPECT().Tools("machine-666").Return(coretools.List{newTools}, nil)

	var diskSpaceStub testing.Stub
	diskSpaceStub.SetErrors(nil, errors.Errorf("full-up"))
	diskSpaceChecked := make(chan struct{}, 1)

	u, err := upgrader.NewAgentUpgrader(upgrader.Config{
		Clock:                       s.clock,
		Logger:                      loggo.GetLogger("test"),
		Client:                      client,
		AgentConfig:                 agentConfig(names.NewMachineTag("666"), s.dataDir),
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
	diskSpaceStub.CheckCall(c, 0, "CheckDiskSpace", s.dataDir, upgrades.MinDiskSpaceMib)
	diskSpaceStub.CheckCall(c, 1, "CheckDiskSpace", os.TempDir(), upgrades.MinDiskSpaceMib)

	_, err = agenttools.ReadTools(s.dataDir, newTools.Version)
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
