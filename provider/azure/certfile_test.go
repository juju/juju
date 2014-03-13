// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io/ioutil"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
)

type certFileSuite struct{}

var _ = gc.Suite(&certFileSuite{})

func (*certFileSuite) TestPathReturnsFullPath(c *gc.C) {
	certFile := tempCertFile{tempDir: "/tmp/dir", filename: "file"}
	c.Check(certFile.Path(), gc.Equals, "/tmp/dir/file")
}

func (*certFileSuite) TestNewTempCertFileCreatesFile(c *gc.C) {
	certData := []byte("content")
	certFile, err := newTempCertFile(certData)
	c.Assert(err, gc.IsNil)
	defer certFile.Delete()

	storedData, err := ioutil.ReadFile(certFile.Path())
	c.Assert(err, gc.IsNil)

	c.Check(storedData, gc.DeepEquals, certData)
}

func (*certFileSuite) TestNewTempCertFileRestrictsAccessToFile(c *gc.C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, gc.IsNil)
	defer certFile.Delete()
	info, err := os.Stat(certFile.Path())
	c.Assert(err, gc.IsNil)
	c.Check(info.Mode().Perm(), gc.Equals, os.FileMode(0600))
}

func (*certFileSuite) TestNewTempCertFileRestrictsAccessToDir(c *gc.C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, gc.IsNil)
	defer certFile.Delete()
	info, err := os.Stat(certFile.tempDir)
	c.Assert(err, gc.IsNil)
	c.Check(info.Mode().Perm(), gc.Equals, os.FileMode(0700))
}

func (*certFileSuite) TestDeleteRemovesFile(c *gc.C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, gc.IsNil)
	certFile.Delete()
	_, err = os.Open(certFile.Path())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (*certFileSuite) TestDeleteIsIdempotent(c *gc.C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, gc.IsNil)
	certFile.Delete()
	certFile.Delete()
	_, err = os.Open(certFile.Path())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}
