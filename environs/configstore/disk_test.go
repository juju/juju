// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&diskInterfaceSuite{})

type diskInterfaceSuite struct {
	interfaceSuite
	dir string
}

func (s *diskInterfaceSuite) SetUpTest(c *gc.C) {
	s.dir = c.MkDir()
	s.NewStore = func(c *gc.C) configstore.Storage {
		store, err := configstore.NewDisk(s.dir)
		c.Assert(err, gc.IsNil)
		return store
	}
}

// storePath returns the path to the environment info
// for the named environment in the given directory.
// If envName is empty, it returns the path
// to the info files' containing directory.
func storePath(dir string, envName string) string {
	path := filepath.Join(dir, "environments")
	if envName != "" {
		path = filepath.Join(path, envName+".jenv")
	}
	return path
}

func (s *diskInterfaceSuite) TearDownTest(c *gc.C) {
	s.NewStore = nil
	// Check that no stray temp files have been left behind
	entries, err := ioutil.ReadDir(storePath(s.dir, ""))
	c.Assert(err, gc.IsNil)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jenv") {
			c.Errorf("found possible stray temp file %q", entry.Name())
		}
	}
}

var _ = gc.Suite(&diskStoreSuite{})

type diskStoreSuite struct {
	testbase.LoggingSuite
}

func (*diskStoreSuite) TestNewDisk(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(filepath.Join(dir, "foo"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	c.Assert(store, gc.IsNil)

	store, err = configstore.NewDisk(filepath.Join(dir))
	c.Assert(err, gc.IsNil)
	c.Assert(store, gc.NotNil)
}

var sampleInfo = `
  user: rog
  password: guessit
  state-servers:
  - example.com
  - kremvax.ru
  ca-cert: 'first line

    second line'
  bootstrap-config:
    secret: blah
    arble: bletch
`[1:]

func (*diskStoreSuite) TestRead(c *gc.C) {
	dir := c.MkDir()
	err := os.Mkdir(storePath(dir, ""), 0700)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(storePath(dir, "someenv"), []byte(sampleInfo), 0666)
	c.Assert(err, gc.IsNil)
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)
	info, err := store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.Initialized(), jc.IsTrue)
	c.Assert(info.APICredentials(), gc.DeepEquals, configstore.APICredentials{
		User:     "rog",
		Password: "guessit",
	})
	c.Assert(info.APIEndpoint(), gc.DeepEquals, configstore.APIEndpoint{
		Addresses: []string{"example.com", "kremvax.ru"},
		CACert:    "first line\nsecond line",
	})
	c.Assert(info.BootstrapConfig(), gc.DeepEquals, map[string]interface{}{
		"secret": "blah",
		"arble":  "bletch",
	})
}

func (*diskStoreSuite) TestReadNotFound(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)
	info, err := store.ReadInfo("someenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	c.Assert(info, gc.IsNil)
}

func (*diskStoreSuite) TestCreate(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	// Create some new environment info.
	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, configstore.APIEndpoint{})
	c.Assert(info.APICredentials(), gc.DeepEquals, configstore.APICredentials{})
	c.Assert(info.Initialized(), jc.IsFalse)
	data, err := ioutil.ReadFile(storePath(dir, "someenv"))
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.HasLen, 0)

	// Check that we can't create it twice.
	info, err = store.CreateInfo("someenv")
	c.Assert(err, gc.Equals, configstore.ErrEnvironInfoAlreadyExists)
	c.Assert(info, gc.IsNil)

	// Check that we can read it again.
	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.Initialized(), jc.IsFalse)
}

func (s *diskStoreSuite) TestCreatePermissions(c *gc.C) {
	// Even though it doesn't test the actual chown, it does test the code path.
	user, err := user.Current()
	c.Assert(err, gc.IsNil)
	s.PatchEnvironment("SUDO_UID", user.Uid)
	s.PatchEnvironment("SUDO_GID", user.Gid)

	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	// Create some new environment info.
	_, err = store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	checkPath := func(path string) {
		stat, err := os.Stat(path)
		c.Assert(err, gc.IsNil)
		c.Assert(fmt.Sprint(stat.Sys().(*syscall.Stat_t).Uid), gc.Equals, user.Uid)
		c.Assert(fmt.Sprint(stat.Sys().(*syscall.Stat_t).Gid), gc.Equals, user.Gid)
	}
	checkPath(storePath(dir, ""))
	checkPath(storePath(dir, "someenv"))
}

func (*diskStoreSuite) TestWriteTempFileFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	// Make the directory non-writable
	err = os.Chmod(storePath(dir, ""), 0555)
	c.Assert(err, gc.IsNil)

	err = info.Write()
	c.Assert(err, gc.ErrorMatches, "cannot create temporary file: .*")

	// Make the directory writable again so that gocheck can clean it up.
	err = os.Chmod(storePath(dir, ""), 0777)
	c.Assert(err, gc.IsNil)
}

func (*diskStoreSuite) TestRenameFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	// Replace the file by an directory which can't be renamed over.
	path := storePath(dir, "someenv")
	err = os.Remove(path)
	c.Assert(err, gc.IsNil)
	err = os.Mkdir(path, 0777)
	c.Assert(err, gc.IsNil)

	err = info.Write()
	c.Assert(err, gc.ErrorMatches, "cannot rename new environment info file: .*")
}

func (*diskStoreSuite) TestDestroyRemovesFiles(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(storePath(dir, "someenv"))
	c.Assert(err, gc.IsNil)

	err = info.Destroy()
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(storePath(dir, "someenv"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "environment info has already been removed")
}
