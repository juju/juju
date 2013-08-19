// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"bytes"
	"io"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/provider/ec2"
	"launchpad.net/juju-core/version"
)

type storageSuite struct {
	storage *testing.EC2HTTPTestStorage
}

var _ = Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *C) {
	var err error

	s.storage, err = testing.NewEC2HTTPTestStorage("127.0.0.1")
	c.Assert(err, IsNil)

	for _, v := range versions {
		s.storage.PutBinary(v)
	}
}

func (s *storageSuite) TearDownTest(c *C) {
	c.Assert(s.storage.Stop(), IsNil)
}

func (s *storageSuite) TestHTTPStorage(c *C) {
	sr := ec2.NewHTTPStorageReader(s.storage.Location())
	list, err := sr.List("tools/juju-")
	c.Assert(err, IsNil)
	c.Assert(len(list), Equals, 6)

	url, err := sr.URL(list[0])
	c.Assert(err, IsNil)
	c.Assert(url, Matches, "http://127.0.0.1:.*/tools/juju-1.0.0-precise-amd64.tgz")

	rc, err := sr.Get(list[0])
	c.Assert(err, IsNil)
	defer rc.Close()

	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, rc)
	c.Assert(err, IsNil)
	c.Assert(buf.String(), Equals, "1.0.0-precise-amd64")
}

var versions = []version.Binary{
	version.MustParseBinary("1.0.0-precise-amd64"),
	version.MustParseBinary("1.0.0-quantal-amd64"),
	version.MustParseBinary("1.0.0-quantal-i386"),
	version.MustParseBinary("1.9.0-quantal-amd64"),
	version.MustParseBinary("1.9.0-precise-i386"),
	version.MustParseBinary("2.0.0-precise-amd64"),
}
