// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"fmt"
	"path/filepath"

	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
)

var _ = gc.Suite(&cacheFileInterfaceSuite{})

type cacheFileInterfaceSuite struct {
	interfaceSuite
	dir   string
	store configstore.Storage
}

func (s *cacheFileInterfaceSuite) SetUpTest(c *gc.C) {
	s.interfaceSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.JES)
	s.dir = c.MkDir()
	s.NewStore = func(c *gc.C) configstore.Storage {
		store, err := configstore.NewDisk(s.dir)
		c.Assert(err, jc.ErrorIsNil)
		return store
	}
	s.store = s.NewStore(c)
}

func (s *cacheFileInterfaceSuite) writeEnv(c *gc.C, name, envUUID, srvUUID, user, password string) configstore.EnvironInfo {
	info := s.store.CreateInfo(name)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"address1", "address2"},
		Hostnames:   []string{"hostname1", "hostname2"},
		CACert:      testing.CACert,
		EnvironUUID: envUUID,
		ServerUUID:  srvUUID,
	})
	info.SetAPICredentials(configstore.APICredentials{
		User:     user,
		Password: password,
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	return info
}

func (s *cacheFileInterfaceSuite) TestServerUUIDWrite(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	info := s.writeEnv(c, "testing", envUUID, envUUID, "tester", "secret")

	// Now make sure the cache file exists and the jenv doesn't
	envDir := filepath.Join(s.dir, "environments")
	filename := configstore.CacheFilename(envDir)
	c.Assert(info.Location(), gc.Equals, fmt.Sprintf("file %q", filename))

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
	c.Assert(cache.Environment, gc.HasLen, 1)
}

func (s *cacheFileInterfaceSuite) TestServerEnvNameExists(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	s.writeEnv(c, "testing", envUUID, envUUID, "tester", "secret")

	info := s.store.CreateInfo("testing")
	// In order to trigger the writing to the cache file, we need to store
	// a server uuid.
	info.SetAPIEndpoint(configstore.APIEndpoint{
		EnvironUUID: envUUID,
		ServerUUID:  envUUID,
	})
	err := info.Write()
	c.Assert(err, gc.ErrorMatches, "environment info already exists")
}

func (s *cacheFileInterfaceSuite) TestWriteServerOnly(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	s.writeEnv(c, "testing", "", envUUID, "tester", "secret")
	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
	c.Assert(cache.Environment, gc.HasLen, 0)
}

func (s *cacheFileInterfaceSuite) TestWriteEnvAfterServer(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	s.writeEnv(c, "testing", "", envUUID, "tester", "secret")
	info := s.store.CreateInfo("testing")

	info.SetAPIEndpoint(configstore.APIEndpoint{
		EnvironUUID: envUUID,
		ServerUUID:  envUUID,
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
	c.Assert(cache.Environment, gc.HasLen, 1)
}

func (s *cacheFileInterfaceSuite) TestWriteDupEnvAfterServer(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	s.writeEnv(c, "testing", "", envUUID, "tester", "secret")
	info := s.store.CreateInfo("testing")

	info.SetAPIEndpoint(configstore.APIEndpoint{
		EnvironUUID: "fake-uuid",
		ServerUUID:  "fake-uuid",
	})
	err := info.Write()
	c.Assert(err, gc.ErrorMatches, "environment info already exists")
}

func (s *cacheFileInterfaceSuite) TestServerUUIDRead(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	s.writeEnv(c, "testing", envUUID, envUUID, "tester", "secret")

	info, err := s.store.ReadInfo("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.APICredentials(), jc.DeepEquals, configstore.APICredentials{
		User:     "tester",
		Password: "secret",
	})
	c.Assert(info.APIEndpoint(), jc.DeepEquals, configstore.APIEndpoint{
		Addresses:   []string{"address1", "address2"},
		Hostnames:   []string{"hostname1", "hostname2"},
		CACert:      testing.CACert,
		EnvironUUID: envUUID,
		ServerUUID:  envUUID,
	})
}

func (s *cacheFileInterfaceSuite) TestServerDetailsShared(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	s.writeEnv(c, "testing", envUUID, envUUID, "tester", "secret")
	info := s.writeEnv(c, "second", "fake-uuid", envUUID, "tester", "new-secret")
	endpoint := info.APIEndpoint()
	endpoint.Addresses = []string{"address2", "address3"}
	endpoint.Hostnames = []string{"hostname2", "hostname3"}
	info.SetAPIEndpoint(endpoint)
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	info, err = s.store.ReadInfo("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.APICredentials(), jc.DeepEquals, configstore.APICredentials{
		User:     "tester",
		Password: "new-secret",
	})
	c.Assert(info.APIEndpoint(), jc.DeepEquals, configstore.APIEndpoint{
		Addresses:   []string{"address2", "address3"},
		Hostnames:   []string{"hostname2", "hostname3"},
		CACert:      testing.CACert,
		EnvironUUID: envUUID,
		ServerUUID:  envUUID,
	})

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
	c.Assert(cache.Environment, gc.HasLen, 2)
}

func (s *cacheFileInterfaceSuite) TestMigrateJENV(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	info := s.writeEnv(c, "testing", envUUID, "", "tester", "secret")
	envDir := filepath.Join(s.dir, "environments")
	jenvFilename := configstore.JENVFilename(envDir, "testing")
	c.Assert(info.Location(), gc.Equals, fmt.Sprintf("file %q", jenvFilename))

	// Add server details and write again will migrate the info to the
	// cache file.
	endpoint := info.APIEndpoint()
	endpoint.ServerUUID = envUUID
	info.SetAPIEndpoint(endpoint)
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(jenvFilename, jc.DoesNotExist)
	cache := s.readCacheFile(c)

	envInfo, ok := cache.Environment["testing"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(envInfo.User, gc.Equals, "tester")
	c.Assert(envInfo.EnvironmentUUID, gc.Equals, envUUID)
	c.Assert(envInfo.ServerUUID, gc.Equals, envUUID)
	// Server entry also written.
	srvInfo, ok := cache.Server["testing"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(srvInfo.User, gc.Equals, "tester")
	c.Assert(srvInfo.ServerUUID, gc.Equals, envUUID)

	readInfo, err := s.store.ReadInfo("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readInfo.APIEndpoint(), jc.DeepEquals, info.APIEndpoint())
}

func (s *cacheFileInterfaceSuite) readCacheFile(c *gc.C) configstore.CacheFile {
	envDir := filepath.Join(s.dir, "environments")
	filename := configstore.CacheFilename(envDir)
	cache, err := configstore.ReadCacheFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	return cache
}

func (s *cacheFileInterfaceSuite) TestExistingJENVBlocksNew(c *gc.C) {
	envUUID := testing.EnvironmentTag.Id()
	info := s.writeEnv(c, "testing", envUUID, "", "tester", "secret")
	envDir := filepath.Join(s.dir, "environments")
	jenvFilename := configstore.JENVFilename(envDir, "testing")
	c.Assert(info.Location(), gc.Equals, fmt.Sprintf("file %q", jenvFilename))

	info = s.store.CreateInfo("testing")
	// In order to trigger the writing to the cache file, we need to store
	// a server uuid.
	info.SetAPIEndpoint(configstore.APIEndpoint{
		EnvironUUID: envUUID,
		ServerUUID:  envUUID,
	})
	err := info.Write()
	c.Assert(err, gc.ErrorMatches, "environment info already exists")
}

func (s *cacheFileInterfaceSuite) TestList(c *gc.C) {
	// List returns both JENV environments and the cache file environments.
	s.writeEnv(c, "jenv-1", "fake-uuid1", "", "tester", "secret")
	s.writeEnv(c, "jenv-2", "fake-uuid2", "", "tester", "secret")
	s.writeEnv(c, "cache-1", "fake-uuid3", "fake-server", "tester", "secret")
	s.writeEnv(c, "cache-2", "fake-uuid4", "fake-server", "tester", "secret")

	environments, err := s.store.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environments, jc.SameContents, []string{"jenv-1", "jenv-2", "cache-1", "cache-2"})

	// Confirm that the sources are from where we'd expect.
	envDir := filepath.Join(s.dir, "environments")
	c.Assert(configstore.JENVFilename(envDir, "jenv-1"), jc.IsNonEmptyFile)
	c.Assert(configstore.JENVFilename(envDir, "jenv-2"), jc.IsNonEmptyFile)
	cache := s.readCacheFile(c)
	names := make([]string, 0)
	for name := range cache.Environment {
		names = append(names, name)
	}
	c.Assert(names, jc.SameContents, []string{"cache-1", "cache-2"})
}

func (s *cacheFileInterfaceSuite) TestDestroy(c *gc.C) {
	info := s.writeEnv(c, "cache-1", "fake-uuid", "fake-server", "tester", "secret")

	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 0)
	c.Assert(cache.ServerData, gc.HasLen, 0)
	c.Assert(cache.Environment, gc.HasLen, 0)
}

func (s *cacheFileInterfaceSuite) TestDestroyTwice(c *gc.C) {
	info := s.writeEnv(c, "cache-1", "fake-uuid", "fake-server", "tester", "secret")

	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "environment info has already been removed")
}

func (s *cacheFileInterfaceSuite) TestDestroyKeepsSharedData(c *gc.C) {
	info := s.writeEnv(c, "cache-1", "fake-uuid1", "fake-server", "tester", "secret")
	s.writeEnv(c, "cache-2", "fake-uuid2", "fake-server", "tester", "secret")

	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 0)
	c.Assert(cache.ServerData, gc.HasLen, 1)
	c.Assert(cache.Environment, gc.HasLen, 1)
}

func (s *cacheFileInterfaceSuite) TestDestroyServerRemovesEnvironments(c *gc.C) {
	// Bit more setup with this test.
	// Create three server references, to two different systems, so we have
	// one system through two different users.
	info := s.writeEnv(c, "cache-1", "fake-server", "fake-server", "tester", "secret")
	s.writeEnv(c, "cache-2", "fake-server", "fake-server", "other", "secret")
	s.writeEnv(c, "cache-3", "fake-server2", "fake-server2", "tester", "secret")

	// And a few environments on each server
	s.writeEnv(c, "cache-4", "fake-env-1", "fake-server", "tester", "secret")
	s.writeEnv(c, "cache-5", "fake-env-2", "fake-server", "other", "secret")
	s.writeEnv(c, "cache-6", "fake-env-3", "fake-server2", "tester", "secret")
	s.writeEnv(c, "cache-7", "fake-env-4", "fake-server2", "tester", "secret")

	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	cache := s.readCacheFile(c)
	c.Assert(cache.Server, gc.HasLen, 1)
	c.Assert(cache.ServerData, gc.HasLen, 1)
	expected := []string{"cache-3", "cache-6", "cache-7"}
	names := []string{}
	for name := range cache.Environment {
		names = append(names, name)
	}
	c.Assert(names, jc.SameContents, expected)
}
