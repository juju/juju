// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
	jc "launchpad.net/juju-core/testing/checkers"
)

var _ = gc.Suite(&diskStoreSuite{})

type diskStoreSuite struct{}

func (*diskStoreSuite) TestNewDisk(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(filepath.Join(dir, "foo", "bar"))
	if false {
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}
	c.Assert(store, gc.IsNil)

	store, err = configstore.NewDisk(filepath.Join(dir, "foo"))
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
`[1:]

func (*diskStoreSuite) TestRead(c *gc.C) {
	dir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(dir, "someenv.yaml"), []byte(sampleInfo), 0666)
	c.Assert(err, gc.IsNil)
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)
	info, err := store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APICredentials(), gc.DeepEquals, environs.APICredentials{
		User:     "rog",
		Password: "guessit",
	})
	c.Assert(info.APIEndpoint(), gc.DeepEquals, environs.APIEndpoint{
		Addresses: []string{"example.com", "kremvax.ru"},
		CACert:    "first line\nsecond line",
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
	c.Assert(info.APIEndpoint(), gc.DeepEquals, environs.APIEndpoint{})
	c.Assert(info.APICredentials(), gc.DeepEquals, environs.APICredentials{})
	data, err := ioutil.ReadFile(filepath.Join(dir, "someenv.yaml"))
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.HasLen, 0)

	// Check that we can't create it twice.
	info, err = store.CreateInfo("someenv")
	c.Assert(err, gc.Equals, environs.ErrEnvironInfoAlreadyExists)
	c.Assert(info, gc.IsNil)

	// Check that it can't be read (pending a Write)
	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.ErrorMatches, "environment in progress.*")
	c.Assert(info, gc.IsNil)

}

func (*diskStoreSuite) TestWrite(c *gc.C) {
	// Also tests SetAPIEndpoint and SetAPICredentials.

	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	// Create the info.
	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	// Set it up with some actual data and write it out.
	expectCreds := environs.APICredentials{
		User:     "foobie",
		Password: "bletch",
	}
	info.SetAPICredentials(expectCreds)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)

	expectEndpoint := environs.APIEndpoint{
		Addresses: []string{"example.com"},
		CACert:    "a cert",
	}
	info.SetAPIEndpoint(expectEndpoint)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, expectEndpoint)

	err = info.Write()
	c.Assert(err, gc.IsNil)

	// Make sure there are no stray files left in the directory.
	infos, err := ioutil.ReadDir(dir)
	c.Assert(err, gc.IsNil)
	c.Assert(infos, gc.HasLen, 1)
	c.Assert(infos[0].Name(), gc.Equals, "someenv.yaml")

	// Check we can read the information back
	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, expectEndpoint)

	// Change the information and write it again.
	expectCreds.User = "arble"
	info.SetAPICredentials(expectCreds)
	err = info.Write()
	c.Assert(err, gc.IsNil)

	// Check we can read the information back
	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)
}

func (*diskStoreSuite) TestWriteTempFileFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	// Make the directory non-writable
	err = os.Chmod(dir, 0555)
	c.Assert(err, gc.IsNil)

	err = info.Write()
	c.Assert(err, gc.ErrorMatches, "cannot create temporary file: .*")

	// Make the directory writable again so that gocheck can clean it up.
	err = os.Chmod(dir, 0777)
	c.Assert(err, gc.IsNil)
}

func (*diskStoreSuite) TestRenameFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	// Replace the file by an directory which can't be renamed over.
	path := filepath.Join(dir, "someenv.yaml")
	err = os.Remove(path)
	c.Assert(err, gc.IsNil)
	err = os.Mkdir(path, 0777)
	c.Assert(err, gc.IsNil)

	err = info.Write()
	c.Assert(err, gc.ErrorMatches, "cannot rename new environment info file: .*")
}

func (*diskStoreSuite) TestDestroy(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, gc.IsNil)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(filepath.Join(dir, "someenv.yaml"))
	c.Assert(err, gc.IsNil)

	err = info.Destroy()
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(filepath.Join(dir, "someenv.yaml"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "environment info has already been removed")
}
