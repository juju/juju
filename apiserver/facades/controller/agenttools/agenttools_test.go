// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

var _ = gc.Suite(&AgentToolsSuite{})

type AgentToolsSuite struct {
	coretesting.BaseSuite
}

type dummyEnviron struct {
	environs.Environ
}

type configGetter struct {
	cfg *config.Config
}

func (s *configGetter) ModelConfig() (*config.Config, error) {
	return s.cfg, nil
}

func (s *AgentToolsSuite) TestCheckTools(c *gc.C) {
	sConfig := coretesting.FakeConfig()
	sConfig = sConfig.Merge(coretesting.Attrs{
		"agent-version": "2.5.0",
	})
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, jc.ErrorIsNil)
	var (
		calledWithMajor, calledWithMinor int
	)
	fakeToolFinder := func(e environs.BootstrapEnviron, maj int, min int, streams []string, filter coretools.Filter) (coretools.List, error) {
		calledWithMajor = maj
		calledWithMinor = min
		ver := version.Binary{Number: version.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		c.Assert(calledWithMajor, gc.Equals, 2)
		c.Assert(calledWithMinor, gc.Equals, 5)
		c.Assert(streams, gc.DeepEquals, []string{"released"})
		return coretools.List{&t}, nil
	}

	ver, err := checkToolsAvailability(getDummyEnviron, cfg, fakeToolFinder)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.Not(gc.Equals), version.Zero)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 0})
}

func (s *AgentToolsSuite) TestCheckToolsNonReleasedStream(c *gc.C) {
	sConfig := coretesting.FakeConfig()
	sConfig = sConfig.Merge(coretesting.Attrs{
		"agent-version": "2.5-alpha1",
		"agent-stream":  "proposed",
	})
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, jc.ErrorIsNil)
	var (
		calledWithMajor, calledWithMinor int
		calledWithStreams                [][]string
	)
	fakeToolFinder := func(e environs.BootstrapEnviron, maj int, min int, streams []string, filter coretools.Filter) (coretools.List, error) {
		calledWithMajor = maj
		calledWithMinor = min
		calledWithStreams = append(calledWithStreams, streams)
		if len(streams) == 1 && streams[0] == "released" {
			return nil, coretools.ErrNoMatches
		}
		ver := version.Binary{Number: version.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		c.Assert(calledWithMajor, gc.Equals, 2)
		c.Assert(calledWithMinor, gc.Equals, 5)
		return coretools.List{&t}, nil
	}
	ver, err := checkToolsAvailability(getDummyEnviron, cfg, fakeToolFinder)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calledWithStreams, gc.DeepEquals, [][]string{{"released"}, {"proposed"}})
	c.Assert(ver, gc.Not(gc.Equals), version.Zero)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 0})
}

type mockState struct {
	configGetter
}

func (e *mockState) Model() (*state.Model, error) {
	return &state.Model{}, nil
}

func (s *AgentToolsSuite) TestUpdateToolsAvailability(c *gc.C) {
	fakeModelConfig := func(_ *state.Model) (*config.Config, error) {
		sConfig := coretesting.FakeConfig()
		sConfig = sConfig.Merge(coretesting.Attrs{
			"agent-version": "2.5.0",
		})
		return config.New(config.NoDefaults, sConfig)
	}
	s.PatchValue(&modelConfig, fakeModelConfig)

	fakeToolFinder := func(_ environs.BootstrapEnviron, _ int, _ int, _ []string, _ coretools.Filter) (coretools.List, error) {
		ver := version.Binary{Number: version.Number{Major: 2, Minor: 5, Patch: 2}}
		olderVer := version.Binary{Number: version.Number{Major: 2, Minor: 5, Patch: 1}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		tOld := coretools.Tools{Version: olderVer, URL: "http://example.com", Size: 1}
		return coretools.List{&t, &tOld}, nil
	}

	var ver version.Number
	fakeUpdate := func(_ *state.Model, v version.Number) error {
		ver = v
		return nil
	}

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	err = updateToolsAvailability(&mockState{configGetter{cfg}}, getDummyEnviron, fakeToolFinder, fakeUpdate)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ver, gc.Not(gc.Equals), version.Zero)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 2})
}

func (s *AgentToolsSuite) TestUpdateToolsAvailabilityNoMatches(c *gc.C) {
	fakeModelConfig := func(_ *state.Model) (*config.Config, error) {
		sConfig := coretesting.FakeConfig()
		sConfig = sConfig.Merge(coretesting.Attrs{
			"agent-version": "2.5.0",
		})
		return config.New(config.NoDefaults, sConfig)
	}
	s.PatchValue(&modelConfig, fakeModelConfig)

	// No new tools available.
	fakeToolFinder := func(_ environs.BootstrapEnviron, _ int, _ int, _ []string, _ coretools.Filter) (coretools.List, error) {
		return nil, errors.NotFoundf("tools")
	}

	// Update should never be called.
	fakeUpdate := func(_ *state.Model, v version.Number) error {
		c.Fail()
		return nil
	}

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	err = updateToolsAvailability(&mockState{configGetter{cfg}}, getDummyEnviron, fakeToolFinder, fakeUpdate)
	c.Assert(err, jc.ErrorIsNil)
}

func getDummyEnviron() (environs.Environ, error) {
	return dummyEnviron{}, nil
}
