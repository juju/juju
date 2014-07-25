// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&diskInterfaceSuite{})

type diskInterfaceSuite struct {
	interfaceSuite
	dir string
}

func (s *diskInterfaceSuite) SetUpTest(c *gc.C) {
	s.interfaceSuite.SetUpTest(c)
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
	s.interfaceSuite.TearDownTest(c)
}

var _ = gc.Suite(&diskStoreSuite{})

type diskStoreSuite struct {
	testing.BaseSuite
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
	c.Assert(info.Location(), gc.Equals, fmt.Sprintf("file %q", dir+"/environments/someenv.jenv"))
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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(info, gc.IsNil)
}

func (*diskStoreSuite) TestWriteFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info := store.CreateInfo("someenv")

	// Make the directory non-writable
	err = os.Chmod(storePath(dir, ""), 0555)
	c.Assert(err, gc.IsNil)

	err = info.Write()
	c.Assert(err, gc.ErrorMatches, ".* permission denied")

	// Make the directory writable again so that gocheck can clean it up.
	err = os.Chmod(storePath(dir, ""), 0777)
	c.Assert(err, gc.IsNil)
}

func (*diskStoreSuite) TestRenameFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	// Replace the file by an directory which can't be renamed over.
	path := storePath(dir, "someenv")
	err = os.Mkdir(path, 0777)
	c.Assert(err, gc.IsNil)

	info := store.CreateInfo("someenv")
	err = info.Write()
	c.Assert(err, gc.ErrorMatches, "environment info already exists")
}

func (*diskStoreSuite) TestDestroyRemovesFiles(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info := store.CreateInfo("someenv")
	err = info.Write()
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

func (*diskStoreSuite) TestWriteSmallerFile(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)
	info := store.CreateInfo("someenv")
	endpoint := configstore.APIEndpoint{
		Addresses:   []string{"this", "is", "never", "validated", "here"},
		EnvironUUID: "90168e4c-2f10-4e9c-83c2-feedfacee5a9",
	}
	info.SetAPIEndpoint(endpoint)
	err = info.Write()
	c.Assert(err, gc.IsNil)

	newInfo, err := store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	// Now change the number of addresses to be shorter.
	endpoint.Addresses = []string{"just one"}
	newInfo.SetAPIEndpoint(endpoint)
	err = newInfo.Write()
	c.Assert(err, gc.IsNil)

	// We should be able to read in in fine.
	yaInfo, err := store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(yaInfo.APIEndpoint().Addresses, gc.DeepEquals, []string{"just one"})
}
