// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"io"
	"io/ioutil"
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type CharmSuite struct{}

var _ = gc.Suite(&CharmSuite{})

func (s *CharmSuite) TestRead(c *gc.C) {
	bPath := testing.Charms.BundlePath(c.MkDir(), "dummy")
	ch, err := charm.Read(bPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
	dPath := testing.Charms.DirPath("dummy")
	ch, err = charm.Read(dPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
}

var inferRepoTests = []struct {
	url  string
	path string
}{
	{"cs:precise/wordpress", ""},
	{"local:oneiric/wordpress", "/some/path"},
}

func (s *CharmSuite) TestInferRepository(c *gc.C) {
	for i, t := range inferRepoTests {
		c.Logf("test %d", i)
		curl, err := charm.InferURL(t.url, "precise")
		c.Assert(err, gc.IsNil)
		repo, err := charm.InferRepository(curl.Reference, "/some/path")
		c.Assert(err, gc.IsNil)
		switch repo := repo.(type) {
		case *charm.LocalRepository:
			c.Assert(repo.Path, gc.Equals, t.path)
		default:
			c.Assert(repo, gc.Equals, charm.Store)
		}
	}
	curl, err := charm.InferURL("local:whatever", "precise")
	c.Assert(err, gc.IsNil)
	_, err = charm.InferRepository(curl.Reference, "")
	c.Assert(err, gc.ErrorMatches, "path to local repository not specified")
	curl.Schema = "foo"
	_, err = charm.InferRepository(curl.Reference, "")
	c.Assert(err, gc.ErrorMatches, "unknown schema for charm reference.*")
}

func checkDummy(c *gc.C, f charm.Charm, path string) {
	c.Assert(f.Revision(), gc.Equals, 1)
	c.Assert(f.Meta().Name, gc.Equals, "dummy")
	c.Assert(f.Config().Options["title"].Default, gc.Equals, "My Title")
	switch f := f.(type) {
	case *charm.Bundle:
		c.Assert(f.Path, gc.Equals, path)
	case *charm.Dir:
		c.Assert(f.Path, gc.Equals, path)
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
