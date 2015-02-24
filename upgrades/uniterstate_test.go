// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type upgradeStateContextSuite struct {
	ctx upgrades.Context

	datadir   string
	statefile *operation.StateFile
}

var _ = gc.Suite(&upgradeStateContextSuite{})

func (s *upgradeStateContextSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("No need to test upgrading state context windows")
	}
	s.datadir = c.MkDir()
	s.statefile = nil
}

func (s *upgradeStateContextSuite) TestContextUpgradeWithMachineTag(c *gc.C) {
	s.initializeContext(c, names.NewMachineTag("0"))
	err := upgrades.AddStoppedFieldToUniterState(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStateContextSuite) TestContextUpgradeWithUnitTag(c *gc.C) {
	s.initializeContext(c, names.NewUnitTag("mysql/0"))
	s.confirmUpgrade(c,
		&operation.State{
			Kind: operation.Continue,
			Step: operation.Pending},
		&operation.State{
			Kind:    operation.Continue,
			Step:    operation.Pending,
			Stopped: true,
		})
}

func (s *upgradeStateContextSuite) TestUpgradeNoStopHookNoChange(c *gc.C) {
	s.initializeContext(c, names.NewUnitTag("mysql/0"))
	given := &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending}
	expect := given
	s.confirmUpgrade(c, given, expect)
}

func (s *upgradeStateContextSuite) TestUpgradeRunHookNoChange(c *gc.C) {
	s.initializeContext(c, names.NewUnitTag("mysql/0"))
	given := &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.Stop,
		}}
	expect := given
	s.confirmUpgrade(c, given, expect)
}

func (s *upgradeStateContextSuite) TestUpgradeInstallOpNoChange(c *gc.C) {
	s.initializeContext(c, names.NewUnitTag("mysql/0"))
	given := &operation.State{
		Kind:     operation.Install,
		Step:     operation.Pending,
		CharmURL: &charm.URL{}}
	expect := given
	s.confirmUpgrade(c, given, expect)
}

func (s *upgradeStateContextSuite) TestUpgradeUpgradeOpNoChange(c *gc.C) {
	s.initializeContext(c, names.NewUnitTag("mysql/0"))
	given := &operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Pending,
		CharmURL: &charm.URL{}}
	expect := given
	s.confirmUpgrade(c, given, expect)
}

func (s *upgradeStateContextSuite) initializeContext(c *gc.C, tag names.Tag) {
	if utag, ok := tag.(names.UnitTag); ok {
		paths := uniter.NewPaths(s.datadir, utag)
		opsfile := paths.State.OperationsFile
		s.statefile = operation.NewStateFile(opsfile)
		c.Assert(os.MkdirAll(filepath.Dir(opsfile), 0755), gc.IsNil)
	}

	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{
			tag:     tag,
			dataDir: s.datadir,
		}}
}

func (s *upgradeStateContextSuite) confirmUpgrade(c *gc.C, given, expect *operation.State) {
	err := s.statefile.Write(given)
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.AddStoppedFieldToUniterState(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	after, err := s.statefile.Read()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(after, jc.DeepEquals, expect)
}
