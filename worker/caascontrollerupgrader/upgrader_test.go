// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caascontrollerupgrader_test

import (
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/caascontrollerupgrader"
	"github.com/juju/juju/worker/gate"
)

type UpgraderSuite struct {
	coretesting.BaseSuite

	confVersion version.Number
	upgrader    *mockUpgrader
	broker      *mockBroker
	ch          chan struct{}

	upgradeStepsComplete gate.Lock
	initialCheckComplete gate.Lock
}

var _ = gc.Suite(&UpgraderSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.upgradeStepsComplete = gate.NewLock()
	s.initialCheckComplete = gate.NewLock()
	s.ch = make(chan struct{})
	s.upgrader = &mockUpgrader{
		watcher: watchertest.NewMockNotifyWatcher(s.ch),
	}
	s.broker = &mockBroker{}
}

func (s *UpgraderSuite) patchVersion(v version.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&series.MustHostSeries, func() string { return v.Series })
	s.PatchValue(&jujuversion.Current, v.Number)
}

func (s *UpgraderSuite) makeUpgrader(c *gc.C) *caascontrollerupgrader.Upgrader {
	w, err := caascontrollerupgrader.NewControllerUpgrader(caascontrollerupgrader.Config{
		Client:                      s.upgrader,
		AgentTag:                    names.NewApplicationTag("app"),
		OrigAgentVersion:            s.confVersion,
		UpgradeStepsWaiter:          s.upgradeStepsComplete,
		InitialUpgradeCheckComplete: s.initialCheckComplete,
		Broker:                      s.broker,
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	s.ch <- struct{}{}
	return w
}

func (s *UpgraderSuite) TestUpgraderSetsVersion(c *gc.C) {
	vers := version.MustParse("6.6.6")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgrader.desired = vers

	u := s.makeUpgrader(c)
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckDone(c)
	c.Assert(s.upgrader.actual.Number, gc.DeepEquals, vers)
}

func (s *UpgraderSuite) TestUpgrader(c *gc.C) {
	vers := version.MustParseBinary("6.6.6-bionic-amd64")
	s.patchVersion(vers)
	s.upgrader.desired = version.MustParse("6.6.7")

	u := s.makeUpgrader(c)
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgrader.actual.Number, gc.DeepEquals, vers.Number)
	s.upgrader.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.upgrader.CheckCall(c, 0, "SetVersion", "application-app", vers)
	s.broker.CheckCallNames(c, "Upgrade")
	s.broker.CheckCall(c, 0, "Upgrade", "controller", s.upgrader.desired)
}

func (s *UpgraderSuite) TestUpgraderDowngradePatch(c *gc.C) {
	vers := version.MustParse("6.6.7")
	s.PatchValue(&jujuversion.Current, vers)
	s.upgrader.desired = version.MustParse("6.6.6")

	u := s.makeUpgrader(c)
	workertest.CleanKill(c, u)

	s.expectInitialUpgradeCheckNotDone(c)
	c.Assert(s.upgrader.actual.Number, gc.DeepEquals, vers)
	s.upgrader.CheckCallNames(c, "SetVersion", "DesiredVersion")
	s.broker.CheckCallNames(c, "Upgrade")
	s.broker.CheckCall(c, 0, "Upgrade", "controller", s.upgrader.desired)
}

func (s *UpgraderSuite) expectInitialUpgradeCheckDone(c *gc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), jc.IsTrue)
}

func (s *UpgraderSuite) expectInitialUpgradeCheckNotDone(c *gc.C) {
	c.Assert(s.initialCheckComplete.IsUnlocked(), jc.IsFalse)
}
