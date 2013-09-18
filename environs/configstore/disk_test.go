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
creds:
  user: rog
  password: guessit
endpoint:
  addresses:
  - example.com
  - kremvax.ru
  cacert: 'first line

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
		CACert:       "first line\nsecond line",
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
