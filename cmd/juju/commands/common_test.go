// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&EnvironFromNameSuite{})

type EnvironFromNameSuite struct {
	coretesting.FakeJujuHomeSuite
}

func (s *EnvironFromNameSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	// Set version.Current to a known value, for which we
	// will make tools available. Individual tests may
	// override this.
	s.PatchValue(&version.Current, v100p64.Number)
	s.PatchValue(&arch.HostArch, func() string { return v100p64.Arch })
	s.PatchValue(&series.HostSeries, func() string { return v100p64.Series })
	s.PatchValue(&os.HostOS, func() os.OSType { return os.Ubuntu })
}

// When attempting to bootstrap, check that when prepare errors out,
// the code cleans up the created jenv file, but *not* any existing
// environment that may have previously been bootstrapped.
func (s *EnvironFromNameSuite) TestCleanup(c *gc.C) {
	destroyedEnvRan := false
	destroyedInfoRan := false

	// Mock functions
	mockDestroyPreparedEnviron := func(
		*cmd.Context,
		environs.Environ,
		configstore.Storage,
		string,
	) {
		destroyedEnvRan = true
	}
	s.PatchValue(&destroyPreparedEnviron, mockDestroyPreparedEnviron)

	mockDestroyEnvInfo := func(
		ctx *cmd.Context,
		cfgName string,
		store configstore.Storage,
		action string,
	) {
		destroyedInfoRan = true
	}
	s.PatchValue(&destroyEnvInfo, mockDestroyEnvInfo)

	mockPrepare := func(
		string,
		environs.BootstrapContext,
		configstore.Storage,
	) (environs.Environ, error) {
		return nil, errors.Errorf("mock-prepare")
	}
	s.PatchValue(&environs.PrepareFromName, mockPrepare)

	s.PatchValue(&environType, func(string) (string, error) { return "", nil })

	ctx := coretesting.Context(c)
	envName := "peckham"
	action := "Bootstrap"
	failer := func(env environs.Environ) error {
		return environs.ErrAlreadyBootstrapped
	}

	// Simulation: prepare should fail and we should only clean up the
	// jenv file. Any existing environment should not be destroyed.
	_, cleanup, err := environFromName(ctx, envName, action, failer)

	c.Check(err, gc.ErrorMatches, ".*mock-prepare$")
	c.Check(destroyedEnvRan, jc.IsFalse)
	c.Check(destroyedInfoRan, jc.IsFalse)

	c.Assert(cleanup, gc.NotNil)
	cleanup()
	c.Check(destroyedEnvRan, jc.IsFalse)
	c.Check(destroyedInfoRan, jc.IsTrue)
}
