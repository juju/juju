// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing/checkers"
	"os"
)

type CertFileSuite struct{}

var _ = Suite(new(CertFileSuite))

func (*CertFileSuite) TestPathReturnsFullPath(c *C) {
	certFile := tempCertFile{tempDir: "/tmp/dir", filename: "file"}
	c.Check(certFile.Path(), Equals, "/tmp/dir/file")
}

func (*CertFileSuite) TestNewTempCertFileCreatesFile(c *C) {
	certData := []byte("content")
	certFile, err := newTempCertFile(certData)
	c.Assert(err, IsNil)
	defer certFile.Delete()

	storedData, err := ioutil.ReadFile(certFile.Path())
	c.Assert(err, IsNil)

	c.Check(storedData, DeepEquals, certData)
}

func (*CertFileSuite) TestNewTempCertFileRestrictsAccessToFile(c *C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, IsNil)
	defer certFile.Delete()
	info, err := os.Stat(certFile.Path())
	c.Assert(err, IsNil)
	c.Check(info.Mode().Perm(), Equals, os.FileMode(0600))
}

func (*CertFileSuite) TestNewTempCertFileRestrictsAccessToDir(c *C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, IsNil)
	defer certFile.Delete()
	info, err := os.Stat(certFile.tempDir)
	c.Assert(err, IsNil)
	c.Check(info.Mode().Perm(), Equals, os.FileMode(0700))
}

func (*CertFileSuite) TestDeleteRemovesFile(c *C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, IsNil)
	certFile.Delete()
	_, err = os.Open(certFile.Path())
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
}

func (*CertFileSuite) TestDeleteIsIdempotent(c *C) {
	certFile, err := newTempCertFile([]byte("content"))
	c.Assert(err, IsNil)
	certFile.Delete()
	certFile.Delete()
	_, err = os.Open(certFile.Path())
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
}
