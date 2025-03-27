// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type StorageSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) TestStorageName(c *gc.C) {
	vers := semversion.MustParseBinary("1.2.3-ubuntu-amd64")
	path := envtools.StorageName(vers, "proposed")
	c.Assert(path, gc.Equals, "tools/proposed/juju-1.2.3-ubuntu-amd64.tgz")
}

func (s *StorageSuite) TestReadListEmpty(c *gc.C) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	_, err = envtools.ReadList(context.Background(), stor, "released", 2, 0)
	c.Assert(err, gc.Equals, envtools.ErrNoTools)
}

func (s *StorageSuite) TestReadList(c *gc.C) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	v100 := semversion.MustParseBinary("1.0.0-ubuntu-amd64")
	v101 := semversion.MustParseBinary("1.0.1-ubuntu-amd64")
	v111 := semversion.MustParseBinary("1.1.1-ubuntu-amd64")
	v201 := semversion.MustParseBinary("2.0.1-ubuntu-amd64")
	agentTools := envtesting.AssertUploadFakeToolsVersions(c, stor, "proposed", "proposed", v100, v101, v111, v201)
	t100 := agentTools[0]
	t101 := agentTools[1]
	t111 := agentTools[2]
	t201 := agentTools[3]

	for i, t := range []struct {
		majorVersion,
		minorVersion int
		list coretools.List
	}{{
		majorVersion: -1, minorVersion: -1, list: coretools.List{t100, t101, t111, t201},
	}, {
		majorVersion: 1, minorVersion: 0, list: coretools.List{t100, t101},
	}, {
		majorVersion: 1, minorVersion: 1, list: coretools.List{t111},
	}, {
		majorVersion: 1, minorVersion: -1, list: coretools.List{t100, t101, t111},
	}, {
		majorVersion: 1, minorVersion: 2, list: nil,
	}, {
		majorVersion: 3, minorVersion: 0, list: nil,
	}} {
		c.Logf("test %d", i)
		list, err := envtools.ReadList(context.Background(), stor, "proposed", t.majorVersion, t.minorVersion)
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
