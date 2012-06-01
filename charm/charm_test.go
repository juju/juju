package charm_test

import (
	"bytes"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/testing"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	TestingT(t)
}

type CharmSuite struct{}

var _ = Suite(&CharmSuite{})

func (s *CharmSuite) TestRead(c *C) {
	bPath := testing.Charms.BundlePath(c.MkDir(), "dummy")
	ch, err := charm.Read(bPath)
	c.Assert(err, IsNil)
	c.Assert(ch.Meta().Name, Equals, "dummy")
	dPath := testing.Charms.DirPath("dummy")
	ch, err = charm.Read(dPath)
	c.Assert(err, IsNil)
	c.Assert(ch.Meta().Name, Equals, "dummy")
}

var inferRepoTests = []struct {
	name   string
	series string
	path   string
	curl   string
}{
	{"wordpress", "precise", "anything", "cs:precise/wordpress"},
	{"oneiric/wordpress", "anything", "anything", "cs:oneiric/wordpress"},
	{"cs:oneiric/wordpress", "anything", "anything", "cs:oneiric/wordpress"},
	{"local:wordpress", "precise", "/some/path", "local:precise/wordpress"},
	{"local:oneiric/wordpress", "anything", "/some/path", "local:oneiric/wordpress"},
}

func (s *CharmSuite) TestInferRepository(c *C) {
	for _, t := range inferRepoTests {
		repo, curl, err := charm.InferRepository(t.name, t.series, t.path)
		c.Assert(err, IsNil)
		expectCurl := charm.MustParseURL(t.curl)
		c.Assert(curl, DeepEquals, expectCurl)
		if localRepo, ok := repo.(*charm.LocalRepository); ok {
			c.Assert(localRepo.Path, Equals, t.path)
			c.Assert(curl.Schema, Equals, "local")
		} else {
			c.Assert(curl.Schema, Equals, "cs")
		}
	}
	_, _, err := charm.InferRepository("local:whatever", "series", "")
	c.Assert(err, ErrorMatches, "path to local repository not specified")
}

func checkDummy(c *C, f charm.Charm, path string) {
	c.Assert(f.Revision(), Equals, 1)
	c.Assert(f.Meta().Name, Equals, "dummy")
	c.Assert(f.Config().Options["title"].Default, Equals, "My Title")
	switch f := f.(type) {
	case *charm.Bundle:
		c.Assert(f.Path, Equals, path)

	case *charm.Dir:
		c.Assert(f.Path, Equals, path)
	}
}

type YamlHacker map[interface{}]interface{}

func ReadYaml(r io.Reader) YamlHacker {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
	m := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, m)
	if err != nil {
		panic(err)
	}
	return YamlHacker(m)
}

func (yh YamlHacker) Reader() io.Reader {
	data, err := goyaml.Marshal(yh)
	if err != nil {
		panic(err)
	}
	return bytes.NewBuffer(data)
}
