// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgrader_test

import (
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/internal/worker/caasupgrader"
	"github.com/juju/juju/internal/worker/gate"
)

type UpgraderSuite struct {
	coretesting.BaseSuite

	confVersion      version.Number
	upgraderClient   *mockUpgraderClient
	operatorUpgrader *mockOperatorUpgrader
	ch               chan struct{}

	upgradeStepsComplete gate.Lock
	initialCheckComplete gate.Lock
}

var _ = gc.Suite(&UpgraderSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.upgradeStepsComplete = gate.NewLock()
	s.initialCheckComplete = gate.NewLock()
	s.ch = make(chan struct{})
	s.upgraderClient = &mockUpgraderClient{
		watcher: watchertest.NewMockNotifyWatcher(s.ch),
	}
	s.operatorUpgrader = &mockOperatorUpgrader{}
}

func (s *UpgraderSuite) patchVersion(v version.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })
	s.PatchValue(&jujuversion.Current, v.Number)
}

func (s *UpgraderSuite) makeUpgrader(c *gc.C, agent names.Tag) *caasupgrader.Upgrader {
	w, err := caasupgrader.NewUpgrader(caasupgrader.Config{
		UpgraderClient:              s.upgraderClient,
		CAASOperatorUpgrader:        s.operatorUpgrader,
		AgentTag:                    agent,
		OrigAgentVersion:            s.confVersion,
		UpgradeStepsWaiter:          s.upgradeStepsComplete,
		InitialUpgradeCheckComplete: s.initialCheckComplete,
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	s.ch <- struct{}{}
	return w
}

func (s *UpgraderSuite) TestUpgraderSetsVersion(c *gc.C) {
	vers := version.MustParse("6.6.6")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgraderClient.desired = vers

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckDone(c)
	c.Assert(s.upgraderClient.actual.Number, gc.DeepEquals, vers)
}

func (s *UpgraderSuite) TestUpgraderController(c *gc.C) {
	vers := version.MustParseBinary("6.6.6-ubuntu-amd64")
	s.patchVersion(vers)
	s.upgraderClient.desired = version.MustParse("6.6.7")

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgraderClient.actual.Number, gc.DeepEquals, vers.Number)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.upgraderClient.CheckCall(c, 0, "SetVersion", "machine-0", vers)
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "machine-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderApplication(c *gc.C) {
	vers := version.MustParseBinary("6.6.6-ubuntu-amd64")
	s.patchVersion(vers)
	s.upgraderClient.desired = version.MustParse("6.6.7")

	u := s.makeUpgrader(c, names.NewApplicationTag("app"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	s.upgraderClient.CheckCallNames(c, "DesiredVersion")
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "application-app", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderSidecarUnit(c *gc.C) {
	vers := version.MustParseBinary("6.6.6-ubuntu-amd64")
	s.patchVersion(vers)
	s.upgraderClient.desired = version.MustParse("6.6.7")

	u := s.makeUpgrader(c, names.NewUnitTag("cockroachdb/0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.upgraderClient.CheckCall(c, 0, "SetVersion", "unit-cockroachdb-0", vers)
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "unit-cockroachdb-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderDowngradePatch(c *gc.C) {
	vers := version.MustParse("6.6.7")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgraderClient.desired = version.MustParse("6.6.6")

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgraderClient.actual.Number, gc.DeepEquals, vers)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "machine-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderDowngradeMinor(c *gc.C) {
	// We'll allow this for the case of restoring a backup from a
	// previous juju version.
	vers := version.MustParse("6.6.7")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgraderClient.desired = version.MustParse("6.5.10")

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgraderClient.actual.Number, gc.DeepEquals, vers)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "machine-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) expectInitialUpgradeCheckDone(c *gc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), jc.IsTrue)
}

func (s *UpgraderSuite) expectInitialUpgradeCheckNotDone(c *gc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), jc.IsFalse)
}
