// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type upgradeStateContextSuite struct {
	datadir   string
	unitTag   names.UnitTag
	statefile *operation.StateFile
}

var _ = gc.Suite(&upgradeStateContextSuite{})

func (s *upgradeStateContextSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("No need to test upgrading state context windows")
	}
	s.datadir = c.MkDir()
	s.statefile = nil
	s.initializeContext(c, names.NewUnitTag("mysql/0"))
}

func (s *upgradeStateContextSuite) TestContextUpgradeWithUnitTag(c *gc.C) {
	given, expectUpgrade :=
		&operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind: hooks.Stop,
			}},
		&operation.State{
			Kind:    operation.Continue,
			Step:    operation.Pending,
			Stopped: true}

	s.confirmUpgrade(c, given, expectUpgrade)
}

func (s *upgradeStateContextSuite) TestUpgradeNoStopHookNoChange(c *gc.C) {
	given := &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending}
	expectNoChange := given

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeRunHookNoChange(c *gc.C) {
	given := &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.Stop,
		}}
	expectNoChange := given

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeInstallOpNoChange(c *gc.C) {
	given := &operation.State{
		Kind:     operation.Install,
		Step:     operation.Pending,
		CharmURL: &charm.URL{}}
	expectNoChange := given

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeUpgradeOpNoChange(c *gc.C) {
	given := &operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Pending,
		CharmURL: &charm.URL{}}
	expectNoChange := given

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeIdempotent(c *gc.C) {
	given, expectUpgrade :=
		&operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind: hooks.Stop,
			}},
		&operation.State{
			Kind:    operation.Continue,
			Step:    operation.Pending,
			Stopped: true}

	s.confirmUpgradeIdempotent(c, given, expectUpgrade)
}

func (s *upgradeStateContextSuite) TestUpgradeMissingStateFile(c *gc.C) {
	s.confirmUniterStateFileMissing(c)
	s.confirmUpgradeNoErrors(c)
	s.confirmUniterStateFileMissing(c)
}

func (s *upgradeStateContextSuite) confirmUpgrade(c *gc.C, given, expect *operation.State) {
	s.writeState(c, given)

	s.confirmUpgradeNoErrors(c)
	s.confirmUniterStateFileMatches(c, expect)
}

func (s *upgradeStateContextSuite) confirmUpgradeIdempotent(c *gc.C, given, expect *operation.State) {
	s.writeState(c, given)

	s.confirmUpgradeNoErrors(c)
	s.confirmUniterStateFileMatches(c, expect)

	s.confirmUpgradeNoErrors(c)
	s.confirmUniterStateFileMatches(c, expect)
}

func (s *upgradeStateContextSuite) writeState(c *gc.C, state *operation.State) {
	err := s.statefile.WriteUnsafe(state)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStateContextSuite) readState(c *gc.C) *operation.State {
	state, err := s.statefile.Read()
	c.Assert(err, jc.ErrorIsNil)
	return state
}

func (s *upgradeStateContextSuite) confirmUpgradeNoErrors(c *gc.C) {
	err := uniter.AddStoppedFieldToUniterState(s.unitTag, s.datadir)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStateContextSuite) confirmUniterStateFileMatches(c *gc.C, expect *operation.State) {
	after := s.readState(c)
	c.Assert(after, jc.DeepEquals, expect)
}

func (s *upgradeStateContextSuite) confirmUniterStateFileMissing(c *gc.C) {
	_, err := s.statefile.Read()
	c.Assert(err, gc.ErrorMatches, "uniter state file does not exist")
}

func (s *upgradeStateContextSuite) initializeContext(c *gc.C, utag names.UnitTag) {
	paths := uniter.NewPaths(s.datadir, utag)
	opsfile := paths.State.OperationsFile
	s.statefile = operation.NewStateFile(opsfile)
	c.Assert(os.MkdirAll(filepath.Dir(opsfile), 0755), gc.IsNil)
	s.unitTag = utag
}
