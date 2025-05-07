// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgrader_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasupgrader"
	"github.com/juju/juju/internal/worker/gate"
)

type UpgraderSuite struct {
	coretesting.BaseSuite

	confVersion      semversion.Number
	upgraderClient   *mockUpgraderClient
	operatorUpgrader *mockOperatorUpgrader
	ch               chan struct{}

	upgradeStepsComplete gate.Lock
	initialCheckComplete gate.Lock
}

var _ = tc.Suite(&UpgraderSuite{})

func (s *UpgraderSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.upgradeStepsComplete = gate.NewLock()
	s.initialCheckComplete = gate.NewLock()
	s.ch = make(chan struct{})
	s.upgraderClient = &mockUpgraderClient{
		watcher: watchertest.NewMockNotifyWatcher(s.ch),
	}
	s.operatorUpgrader = &mockOperatorUpgrader{}
}

func (s *UpgraderSuite) patchVersion(v semversion.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })
	s.PatchValue(&jujuversion.Current, v.Number)
}

func (s *UpgraderSuite) makeUpgrader(c *tc.C, agent names.Tag) *caasupgrader.Upgrader {
	w, err := caasupgrader.NewUpgrader(caasupgrader.Config{
		UpgraderClient:              s.upgraderClient,
		CAASOperatorUpgrader:        s.operatorUpgrader,
		AgentTag:                    agent,
		OrigAgentVersion:            s.confVersion,
		UpgradeStepsWaiter:          s.upgradeStepsComplete,
		InitialUpgradeCheckComplete: s.initialCheckComplete,
	})
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	s.ch <- struct{}{}
	return w
}

func (s *UpgraderSuite) TestUpgraderSetsVersion(c *tc.C) {
	vers := semversion.MustParse("6.6.6")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgraderClient.desired = vers

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckDone(c)
	c.Assert(s.upgraderClient.actual.Number, tc.DeepEquals, vers)
}

func (s *UpgraderSuite) TestUpgraderController(c *tc.C) {
	vers := semversion.MustParseBinary("6.6.6-ubuntu-amd64")
	s.patchVersion(vers)
	s.upgraderClient.desired = semversion.MustParse("6.6.7")

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgraderClient.actual.Number, tc.DeepEquals, vers.Number)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.upgraderClient.CheckCall(c, 0, "SetVersion", "machine-0", vers)
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "machine-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderApplication(c *tc.C) {
	vers := semversion.MustParseBinary("6.6.6-ubuntu-amd64")
	s.patchVersion(vers)
	s.upgraderClient.desired = semversion.MustParse("6.6.7")

	u := s.makeUpgrader(c, names.NewApplicationTag("app"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	s.upgraderClient.CheckCallNames(c, "DesiredVersion")
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "application-app", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderSidecarUnit(c *tc.C) {
	vers := semversion.MustParseBinary("6.6.6-ubuntu-amd64")
	s.patchVersion(vers)
	s.upgraderClient.desired = semversion.MustParse("6.6.7")

	u := s.makeUpgrader(c, names.NewUnitTag("cockroachdb/0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.upgraderClient.CheckCall(c, 0, "SetVersion", "unit-cockroachdb-0", vers)
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "unit-cockroachdb-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderDowngradePatch(c *tc.C) {
	vers := semversion.MustParse("6.6.7")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgraderClient.desired = semversion.MustParse("6.6.6")

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgraderClient.actual.Number, tc.DeepEquals, vers)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "machine-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) TestUpgraderDowngradeMinor(c *tc.C) {
	// We'll allow this for the case of restoring a backup from a
	// previous juju version.
	vers := semversion.MustParse("6.6.7")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgraderClient.desired = semversion.MustParse("6.5.10")

	u := s.makeUpgrader(c, names.NewMachineTag("0"))
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgraderClient.actual.Number, tc.DeepEquals, vers)
	s.upgraderClient.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.operatorUpgrader.CheckCallNames(c, "Upgrade")
	s.operatorUpgrader.CheckCall(c, 0, "Upgrade", "machine-0", s.upgraderClient.desired)
}

func (s *UpgraderSuite) expectInitialUpgradeCheckDone(c *tc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), tc.IsTrue)
}

func (s *UpgraderSuite) expectInitialUpgradeCheckNotDone(c *tc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), tc.IsFalse)
}
