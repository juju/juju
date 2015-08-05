// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package newtoolsversionchecker_test

import (
	//stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/newtoolsversionchecker"
)

var _ = gc.Suite(&ToolsCheckerSuite{})

type ToolsCheckerSuite struct {
	coretesting.BaseSuite
}

func (s *ToolsCheckerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *ToolsCheckerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *ToolsCheckerSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *ToolsCheckerSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

type dummyEnviron struct {
	environs.Environ
}

func (dummyEnviron) Config() *config.Config {
	sConfig := dummy.SampleConfig()
	sConfig["agent-version"] = "2.5.0"
	c, err := config.New(config.NoDefaults, sConfig)
	if err != nil {
		panic("cannot make a proper config")
	}
	return c
}

type envCapable struct {
}

func (e *envCapable) Environment() (*state.Environment, error) {
	return &state.Environment{}, nil
}

func (s *ToolsCheckerSuite) TestWorker(c *gc.C) {
	fakeNewEnvirons := func(*config.Config) (environs.Environ, error) {
		c.Log("entered fake new environ")
		return &dummyEnviron{}, nil
	}
	s.PatchValue(newtoolsversionchecker.NewEnvirons, fakeNewEnvirons)

	fakeEnvConfig := func(_ *state.Environment) (*config.Config, error) {
		sConfig := dummy.SampleConfig()
		sConfig["agent-version"] = "2.5.0"
		return config.New(config.NoDefaults, sConfig)
	}
	s.PatchValue(newtoolsversionchecker.EnvConfig, fakeEnvConfig)

	params := &newtoolsversionchecker.VersionCheckerParams{
		CheckInterval: coretesting.ShortWait,
	}

	var sookMajor, sookMinor int
	findTools := func(_ environs.Environ, maj int, min int, _ coretools.Filter) (coretools.List, error) {
		sookMajor = maj
		sookMinor = min
		ver := version.Binary{Number: version.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		return coretools.List{&t}, nil
	}

	var foundVersion version.Number
	ranUpdateVersion := make(chan bool, 1)
	envVersionUpdate := func(_ *state.Environment, ver version.Number) error {
		foundVersion = ver
		defer func() { ranUpdateVersion <- true }()
		return nil
	}

	checker := newtoolsversionchecker.NewForTests(
		&envCapable{},
		params,
		findTools,
		envVersionUpdate,
	)
	s.AddCleanup(func(*gc.C) {
		checker.Kill()
		c.Assert(checker.Wait(), jc.ErrorIsNil)
	})

	select {
	case <-ranUpdateVersion:
		c.Assert(sookMajor, gc.Equals, 2)
		c.Assert(sookMinor, gc.Equals, 5)
		ver := version.Number{Major: 2, Minor: 5}
		c.Assert(foundVersion, gc.Equals, ver)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting worker to seek new tool versions")
	}

}
