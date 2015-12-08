// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type preupgradechecksSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&preupgradechecksSuite{})

func (s *preupgradechecksSuite) TestCheckFreeDiskSpace(c *gc.C) {
	// Expect an impossibly large amount of free disk.
	s.PatchValue(upgrades.MinDiskSpaceGib, 1000*humanize.EiByte/humanize.GiByte)
	err := upgrades.PreUpgradeSteps(nil, &mockAgentConfig{dataDir: "/"}, false)
	c.Assert(err, gc.ErrorMatches, "not enough free disk space for upgrade .*")
}

type mockEnviron struct {
	environs.Environ
	allInstancesCalled bool
}

func (m *mockEnviron) AllInstances() ([]instance.Instance, error) {
	m.allInstancesCalled = true
	return nil, errors.New("instances error")
}

func (s *preupgradechecksSuite) TestCheckProviderAPI(c *gc.C) {
	// We don't want to fail on disk space in this test.
	s.PatchValue(upgrades.MinDiskSpaceGib, 0)

	env := &mockEnviron{}
	var callSt *state.State
	s.PatchValue(upgrades.GetEnvironment, func(st *state.State) (environs.Environ, error) {
		c.Assert(st, gc.Equals, callSt)
		return env, nil
	})
	err := upgrades.PreUpgradeSteps(callSt, &mockAgentConfig{dataDir: "/"}, true)
	c.Assert(err, gc.ErrorMatches, "cannot make API call to provider: instances error")
	c.Assert(env.allInstancesCalled, jc.IsTrue)
}

func (s *preupgradechecksSuite) TestCheckProviderAPINotMaster(c *gc.C) {
	// We don't want to fail on disk space in this test.
	s.PatchValue(upgrades.MinDiskSpaceGib, 0)

	env := &mockEnviron{}
	var callSt *state.State
	s.PatchValue(upgrades.GetEnvironment, func(st *state.State) (environs.Environ, error) {
		c.Assert(st, gc.Equals, callSt)
		return env, nil
	})
	err := upgrades.PreUpgradeSteps(callSt, &mockAgentConfig{dataDir: "/"}, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.allInstancesCalled, jc.IsFalse)
}
