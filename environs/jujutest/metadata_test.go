package jujutest

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
)

type metadataSuite struct{}

var _ = Suite(&metadataSuite{})

func (s *metadataSuite) TestInitVFS(c *C) {
	vfs := NewVFS([]FileContent{{"a", "a-content"}, {"b", "b-content"}})
	c.Assert(vfs, NotNil)
	f, err := vfs.Open("a")
	c.Assert(f, NotNil)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(f)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "a-content")
	c.Assert(f.Close(), IsNil)
}

func (s *metadataSuite) TestVFSNoFile(c *C) {
	vfs := NewVFS([]FileContent{{"a", "a-content"}})
	f, err := vfs.Open("no-such-file")
	c.Assert(f, IsNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}
