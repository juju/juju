// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

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
		c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jenv") && entry.Name() != "cache.yaml" {
			c.Errorf("found possible stray temp file %s, %q", s.dir, entry.Name())
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store, gc.NotNil)
}

var sampleInfo = `
  user: rog
  password: guessit
  state-servers:
  - 10.0.0.1
  - 127.0.0.1
  server-hostnames:
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
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(storePath(dir, "someenv"), []byte(sampleInfo), 0666)
	c.Assert(err, jc.ErrorIsNil)
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)
	info, err := store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Initialized(), jc.IsTrue)
	c.Assert(info.APICredentials(), gc.DeepEquals, configstore.APICredentials{
		User:     "rog",
		Password: "guessit",
	})
	c.Assert(info.APIEndpoint(), gc.DeepEquals, configstore.APIEndpoint{
		Addresses: []string{"10.0.0.1", "127.0.0.1"},
		Hostnames: []string{"example.com", "kremvax.ru"},
		CACert:    "first line\nsecond line",
	})
	c.Assert(info.Location(), gc.Equals, fmt.Sprintf("file %q", filepath.Join(dir, "environments", "someenv.jenv")))
	c.Assert(info.BootstrapConfig(), gc.DeepEquals, map[string]interface{}{
		"secret": "blah",
		"arble":  "bletch",
	})
}

func (*diskStoreSuite) TestReadNotFound(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)
	info, err := store.ReadInfo("someenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(info, gc.IsNil)
}

func (*diskStoreSuite) TestWriteFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)

	info := store.CreateInfo("someenv")

	// Make the directory non-writable
	err = os.Chmod(storePath(dir, ""), 0555)
	c.Assert(err, jc.ErrorIsNil)

	// Cannot use permissions properly on windows for now
	if runtime.GOOS != "windows" {
		err = info.Write()
		c.Assert(err, gc.ErrorMatches, ".* permission denied")
	}

	// Make the directory writable again so that gocheck can clean it up.
	err = os.Chmod(storePath(dir, ""), 0777)
	c.Assert(err, jc.ErrorIsNil)
}

func (*diskStoreSuite) TestRenameFails(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: the way the error is checked doesn't work on windows")
	}
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)

	// Replace the file by an directory which can't be renamed over.
	path := storePath(dir, "someenv")
	err = os.Mkdir(path, 0777)
	c.Assert(err, jc.ErrorIsNil)

	info := store.CreateInfo("someenv")
	err = info.Write()
	c.Assert(err, gc.ErrorMatches, "environment info already exists")
}

func (*diskStoreSuite) TestDestroyRemovesFiles(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)

	info := store.CreateInfo("someenv")
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(storePath(dir, "someenv"))
	c.Assert(err, jc.ErrorIsNil)

	err = info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(storePath(dir, "someenv"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "environment info has already been removed")
}

func (*diskStoreSuite) TestWriteSmallerFile(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)
	info := store.CreateInfo("someenv")
	endpoint := configstore.APIEndpoint{
		Addresses:   []string{"this", "is", "never", "validated", "here"},
		Hostnames:   []string{"neither", "is", "this"},
		EnvironUUID: testing.EnvironmentTag.Id(),
	}
	info.SetAPIEndpoint(endpoint)
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	newInfo, err := store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	// Now change the number of addresses to be shorter.
	endpoint.Addresses = []string{"just one"}
	endpoint.Hostnames = []string{"just this"}
	newInfo.SetAPIEndpoint(endpoint)
	err = newInfo.Write()
	c.Assert(err, jc.ErrorIsNil)

	// We should be able to read in in fine.
	yaInfo, err := store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(yaInfo.APIEndpoint().Addresses, gc.DeepEquals, []string{"just one"})
	c.Assert(yaInfo.APIEndpoint().Hostnames, gc.DeepEquals, []string{"just this"})
}

func (*diskStoreSuite) TestConcurrentAccess(c *gc.C) {
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("test-log", &tw, loggo.DEBUG), gc.IsNil)

	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)

	envDir := storePath(dir, "")
	lock, err := configstore.AcquireEnvironmentLock(envDir, "blocking-op")
	c.Assert(err, jc.ErrorIsNil)
	defer lock.Unlock()

	_, err = store.ReadInfo("someenv")
	c.Assert(errors.Cause(err), gc.Equals, fslock.ErrTimeout)

	// Using . between environments and env.lock so we don't have to care
	// about forward vs. backwards slash separator.
	messages := []jc.SimpleMessage{
		{loggo.WARNING, `configstore lock held, lock dir: .*environments.env\.lock`},
		{loggo.WARNING, `lock holder message: pid: \d+, operation: blocking-op`},
	}

	c.Check(tw.Log(), jc.LogMatches, messages)
}
