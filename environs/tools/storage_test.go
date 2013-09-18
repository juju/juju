// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type StorageSuite struct {
	env environs.Environ
	testing.LoggingSuite
	dataDir string
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	s.env, err = environs.Prepare(cfg)
	c.Assert(err, gc.IsNil)
	s.dataDir = c.MkDir()
}

func (s *StorageSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (s *StorageSuite) TestStorageName(c *gc.C) {
	vers := version.MustParseBinary("1.2.3-precise-amd64")
	path := envtools.StorageName(vers)
	c.Assert(path, gc.Equals, "tools/juju-1.2.3-precise-amd64.tgz")
}

func (s *StorageSuite) TestSetToolPrefix(c *gc.C) {
	vers := version.MustParseBinary("1.2.3-precise-amd64")
	reset := envtools.SetToolPrefix("test_prefix/juju-")
	path := envtools.StorageName(vers)
	c.Assert(path, gc.Equals, "test_prefix/juju-1.2.3-precise-amd64.tgz")
	reset()
	path = envtools.StorageName(vers)
	c.Assert(path, gc.Equals, "tools/juju-1.2.3-precise-amd64.tgz")
}

func (s *StorageSuite) TestReadListEmpty(c *gc.C) {
	store := s.env.Storage()
	_, err := envtools.ReadList(store, 2, 0)
	c.Assert(err, gc.Equals, envtools.ErrNoTools)
}

func (s *StorageSuite) assertReadList(c *gc.C) {
	defer func() {
		envtools.SetToolPrefix(envtools.DefaultToolPrefix)
	}()
	store := s.env.Storage()
	v001 := version.MustParseBinary("0.0.1-precise-amd64")
	t001 := envtesting.UploadFakeToolsVersion(c, store, v001)
	v100 := version.MustParseBinary("1.0.0-precise-amd64")
	t100 := envtesting.UploadFakeToolsVersion(c, store, v100)
	v101 := version.MustParseBinary("1.0.1-precise-amd64")
	t101 := envtesting.UploadFakeToolsVersion(c, store, v101)
	v111 := version.MustParseBinary("1.1.1-precise-amd64")
	t111 := envtesting.UploadFakeToolsVersion(c, store, v111)

	for i, t := range []struct {
		majorVersion,
		minorVersion int
		list coretools.List
	}{{
		0, 0, coretools.List{t001},
	}, {
		1, 0, coretools.List{t100, t101},
	}, {
		1, 1, coretools.List{t111},
	}, {
		1, -1, coretools.List{t100, t101, t111},
	}, {
		1, 2, nil,
	}, {
		2, 0, nil,
	}} {
		c.Logf("test %d", i)
		list, err := envtools.ReadList(store, t.majorVersion, t.minorVersion)
		if t.list != nil {
			c.Assert(err, gc.IsNil)
			c.Assert(list, gc.DeepEquals, t.list)
		} else {
			c.Assert(err, gc.Equals, coretools.ErrNoMatches)
		}
	}
}

func (s *StorageSuite) TestReadListLegacyLocation(c *gc.C) {
	envtools.SetToolPrefix(envtools.DefaultToolPrefix)
	s.assertReadList(c)
}

func (s *StorageSuite) TestReadList(c *gc.C) {
	envtools.SetToolPrefix(envtools.NewToolPrefix)
	s.assertReadList(c)
}

var setenvTests = []struct {
	set    string
	expect []string
}{
	{"foo=1", []string{"foo=1", "arble="}},
	{"foo=", []string{"foo=", "arble="}},
	{"arble=23", []string{"foo=bar", "arble=23"}},
	{"zaphod=42", []string{"foo=bar", "arble=", "zaphod=42"}},
}

func (*StorageSuite) TestSetenv(c *gc.C) {
	env0 := []string{"foo=bar", "arble="}
	for i, t := range setenvTests {
		c.Logf("test %d", i)
		env := make([]string, len(env0))
		copy(env, env0)
		env = envtools.Setenv(env, t.set)
		c.Check(env, gc.DeepEquals, t.expect)
	}
}
