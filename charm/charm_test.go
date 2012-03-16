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

type S struct {
	repo *testing.Repo
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	s.repo = testing.NewRepo(c)
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
