package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"os"
	"path"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ConnSuite struct{}

var _ = Suite(&ConnSuite{})

func (s *ConnSuite) TestNewConn(c *C) {
	home := c.MkDir()
	os.Setenv("HOME", home)
	conn, err := juju.NewConn("")
	c.Assert(conn, IsNil)
	c.Assert(err, ErrorMatches, ".*: no such file or directory")

	os.Mkdir(path.Join(home, ".juju"), 0755)
	envs := path.Join(home, ".juju", "environments.yaml")
	ioutil.WriteFile(envs, []byte(`
default:
    erewhemos
environments:
    erewhon:
        type: dummy
    erewhemos:
        type: dummy
`), 0644)

	// Nasty hackish testing; inferring correct environ choice from errors
	// caused by lack of registered providers. Will need to change if/when
	// we get a globaly available dummy provider.
	conn, err = juju.NewConn("")
	c.Assert(err, ErrorMatches, `environment "erewhemos" has an unknown provider type: "dummy"`)
	c.Assert(conn, IsNil)

	conn, err = juju.NewConn("erewhon")
	c.Assert(err, ErrorMatches, `environment "erewhon" has an unknown provider type: "dummy"`)
	c.Assert(conn, IsNil)

	conn, err = juju.NewConn("entstixenon")
	c.Assert(err, ErrorMatches, `unknown environment "entstixenon"`)
	c.Assert(conn, IsNil)
}
