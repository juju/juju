// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"os"
	"path/filepath"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type upgradeStateContextSuite struct {
	datadir         string
	unitTag         names.UnitTag
	uniterStateFile string
	statefile       *operation.StateFile
}

var _ = gc.Suite(&upgradeStateContextSuite{})

func (s *upgradeStateContextSuite) SetUpTest(c *gc.C) {
	s.datadir = c.MkDir()
	s.statefile = nil
	s.initializeContext(c, names.NewUnitTag("mysql/0"))
}

func (s *upgradeStateContextSuite) TestContextUpgradeWithUnitTag(c *gc.C) {
	given, expectUpgrade :=
		&oldState{
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
	given, expectNoChange := &oldState{
		Kind: operation.Continue,
		Step: operation.Pending,
	}, &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeRunHookNoChange(c *gc.C) {
	given, expectNoChange := &oldState{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.Stop,
		},
	}, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hooks.Stop,
		},
	}

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeInstallOpNoChange(c *gc.C) {
	given, expectNoChange := &oldState{
		Kind:     operation.Install,
		Step:     operation.Pending,
		CharmURL: &charm.URL{},
	}, &operation.State{
		Kind:     operation.Install,
		Step:     operation.Pending,
		CharmURL: &charm.URL{},
	}

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeUpgradeOpNoChange(c *gc.C) {
	given, expectNoChange := &oldState{
		Kind:     operation.Upgrade,
		Step:     operation.Pending,
		CharmURL: &charm.URL{},
	}, &operation.State{
		Kind:     operation.Upgrade,
		Step:     operation.Pending,
		CharmURL: &charm.URL{},
	}

	s.confirmUpgrade(c, given, expectNoChange)
}

func (s *upgradeStateContextSuite) TestUpgradeIdempotent(c *gc.C) {
	given, expectUpgrade :=
		&oldState{
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

func (s *upgradeStateContextSuite) confirmUpgrade(c *gc.C, given *oldState, expect *operation.State) {
	s.writeOldState(c, given)

	s.confirmUpgradeNoErrors(c)
	s.confirmUniterStateFileMatches(c, expect)
}

func (s *upgradeStateContextSuite) confirmUpgradeIdempotent(c *gc.C, given *oldState, expect *operation.State) {
	s.writeOldState(c, given)

	s.confirmUpgradeNoErrors(c)
	s.confirmUniterStateFileMatches(c, expect)

	s.confirmUpgradeNoErrors(c)
	s.confirmUniterStateFileMatches(c, expect)
}

func (s *upgradeStateContextSuite) writeOldState(c *gc.C, state *oldState) {
	err := utils.WriteYaml(s.uniterStateFile, state)
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
	s.uniterStateFile = paths.State.OperationsFile
	s.statefile = operation.NewStateFile(s.uniterStateFile)
	c.Assert(os.MkdirAll(filepath.Dir(s.uniterStateFile), 0755), gc.IsNil)
	s.unitTag = utag
}

// oldState is a surrogate type to imitate the relevant parts of the
// pre-1.23 operation.State struct.
type oldState struct {
	Kind     operation.Kind `yaml:"op"`
	Step     operation.Step `yaml:"opstep"`
	Hook     *hook.Info     `yaml:"hook,omitempty"`
	CharmURL *charm.URL     `yaml:"charm,omitempty"`
}
