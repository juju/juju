// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&cacheFileInterfaceSuite{})

type cacheFileInterfaceSuite struct {
	interfaceSuite
	dir   string
	store configstore.Storage
}

func (s *cacheFileInterfaceSuite) SetUpTest(c *gc.C) {
	s.interfaceSuite.SetUpTest(c)
	s.dir = c.MkDir()
	s.NewStore = func(c *gc.C) configstore.Storage {
		store, err := configstore.NewDisk(s.dir)
		c.Assert(err, jc.ErrorIsNil)
		return store
	}
	s.store = s.NewStore(c)
}

func (s *cacheFileInterfaceSuite) writeEnv(c *gc.C, name, srvUUID, user, password string) configstore.EnvironInfo {
	info := s.store.CreateInfo(srvUUID, name)
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	return info
}

func (s *cacheFileInterfaceSuite) TestServerUUIDWrite(c *gc.C) {
	modelUUID := testing.ModelTag.Id()
	info := s.writeEnv(c, "testing", modelUUID, "tester", "secret")

	// Now make sure the cache file exists
	filename := configstore.CacheFilename(s.dir)
	c.Assert(info.Location(), gc.Equals, fmt.Sprintf("file %q", filename))

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
}

func (s *cacheFileInterfaceSuite) TestWriteServerOnly(c *gc.C) {
	s.writeEnv(c, "testing", "", "tester", "secret")
	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
}

func (s *cacheFileInterfaceSuite) readCacheFile(c *gc.C) configstore.CacheFile {
	filename := configstore.CacheFilename(s.dir)
	cache, err := configstore.ReadCacheFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	return cache
}

func (s *cacheFileInterfaceSuite) TestDestroy(c *gc.C) {
	info := s.writeEnv(c, "cache-1", "fake-server", "tester", "secret")

	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 0)
	c.Assert(cache.ServerData, gc.HasLen, 0)
}

func (s *cacheFileInterfaceSuite) TestDestroyTwice(c *gc.C) {
	info := s.writeEnv(c, "cache-1", "fake-server", "tester", "secret")

	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "model info has already been removed")
}

func (s *cacheFileInterfaceSuite) TestDestroyKeepsOtherModels(c *gc.C) {
	info := s.writeEnv(c, "cache-1", "fake-server1", "tester", "secret")
	s.writeEnv(c, "cache-2", "fake-server2", "tester", "secret")

	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
}
