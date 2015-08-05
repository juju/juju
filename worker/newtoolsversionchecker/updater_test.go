// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package newtoolsversionchecker

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type UpdaterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UpdaterSuite{})

func (s *UpdaterSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *UpdaterSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *UpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *UpdaterSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

type dummyEnviron struct {
	environs.Environ
}

func (s *UpdaterSuite) TestCheckTools(c *gc.C) {
	sConfig := dummy.SampleConfig()
	sConfig["agent-version"] = "2.5.0"
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, jc.ErrorIsNil)
	fakeNewEnvirons := func(*config.Config) (environs.Environ, error) {
		return dummyEnviron{}, nil
	}
	s.PatchValue(&newEnvirons, fakeNewEnvirons)
	var (
		cEnv       environs.Environ
		cMaj, cMin int
		cFilter    coretools.Filter
	)
	fakeToolFinder := func(e environs.Environ, maj int, min int, filter coretools.Filter) (coretools.List, error) {
		cEnv = e
		cMaj = maj
		cMin = min
		cFilter = filter
		ver := version.Binary{Number: version.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		return coretools.List{&t}, nil
	}

	ver, err := checkToolsAvailability(cfg, fakeToolFinder)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cMaj, gc.Equals, 2)
	c.Assert(cMin, gc.Equals, 5)
	c.Assert(ver, gc.Not(gc.Equals), version.Zero)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 0})
}

type envCapable struct {
}

func (e *envCapable) Environment() (*state.Environment, error) {
	return &state.Environment{}, nil
}

func (s *UpdaterSuite) TestUpdateToolsAvailability(c *gc.C) {
	fakeNewEnvirons := func(*config.Config) (environs.Environ, error) {
		return dummyEnviron{}, nil
	}
	s.PatchValue(&newEnvirons, fakeNewEnvirons)

	fakeEnvConfig := func(_ *state.Environment) (*config.Config, error) {
		sConfig := dummy.SampleConfig()
		sConfig["agent-version"] = "2.5.0"
		return config.New(config.NoDefaults, sConfig)
	}
	s.PatchValue(&envConfig, fakeEnvConfig)

	fakeToolFinder := func(_ environs.Environ, _ int, _ int, _ coretools.Filter) (coretools.List, error) {
		ver := version.Binary{Number: version.Number{Major: 2, Minor: 5}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		return coretools.List{&t}, nil
	}

	var ver version.Number
	fakeUpdate := func(_ *state.Environment, v version.Number) error {
		ver = v
		return nil
	}

	err := updateToolsAvailability(&envCapable{}, fakeToolFinder, fakeUpdate)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ver, gc.Not(gc.Equals), version.Zero)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 0})
}
