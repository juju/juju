// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type StorageSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) TestStorageName(c *gc.C) {
	vers := version.MustParseBinary("1.2.3-precise-amd64")
	path := envtools.StorageName(vers, "proposed")
	c.Assert(path, gc.Equals, "tools/proposed/juju-1.2.3-precise-amd64.tgz")
}

func (s *StorageSuite) TestReadListEmpty(c *gc.C) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	_, err = envtools.ReadList(stor, "released", 2, 0)
	c.Assert(err, gc.Equals, envtools.ErrNoTools)
}

func (s *StorageSuite) TestReadList(c *gc.C) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	v001 := version.MustParseBinary("0.0.1-precise-amd64")
	v100 := version.MustParseBinary("1.0.0-precise-amd64")
	v101 := version.MustParseBinary("1.0.1-precise-amd64")
	v111 := version.MustParseBinary("1.1.1-precise-amd64")
	agentTools := envtesting.AssertUploadFakeToolsVersions(c, stor, "proposed", "proposed", v001, v100, v101, v111)
	t001 := agentTools[0]
	t100 := agentTools[1]
	t101 := agentTools[2]
	t111 := agentTools[3]

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
		list, err := envtools.ReadList(stor, "proposed", t.majorVersion, t.minorVersion)
		if t.list != nil {
			c.Assert(err, jc.ErrorIsNil)
			// ReadList doesn't set the Size or SHA256, so blank out those attributes.
			for _, tool := range t.list {
				tool.Size = 0
				tool.SHA256 = ""
			}
			c.Assert(list, gc.DeepEquals, t.list)
		} else {
			c.Assert(err, gc.Equals, coretools.ErrNoMatches)
		}
	}
}

func (s *StorageSuite) TestReadListLegacyPPC64(c *gc.C) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	v100 := version.MustParseBinary("1.0.0-precise-amd64")
	v101 := version.MustParseBinary("1.0.1-precise-ppc64el")
	agentTools := envtesting.AssertUploadFakeToolsVersions(c, stor, "proposed", "proposed", v100, v101)

	amd64Tools := agentTools[0]
	ppc64elTools := agentTools[1]
	// We also expect metadata for ppc64 to be added.
	ppc64Tools := *ppc64elTools
	ppc64Tools.Version.Arch = "ppc64"
	expected := coretools.List{amd64Tools, ppc64elTools, &ppc64Tools}

	list, err := envtools.ReadList(stor, "proposed", 1, 0)
	c.Assert(err, jc.ErrorIsNil)
	// ReadList doesn't set the Size or SHA256, so blank out those attributes.
	for _, tool := range expected {
		tool.Size = 0
		tool.SHA256 = ""
	}
	c.Assert(list, gc.DeepEquals, expected)
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
