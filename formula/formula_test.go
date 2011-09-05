package formula_test

import (
	"testing"
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
)


func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

func (s *S) TestParseId(c *C) {
	namespace, name, rev, err := formula.ParseId("local:mysql-21")
	c.Assert(err, IsNil)
	c.Assert(namespace, Equals, "local")
	c.Assert(name, Equals, "mysql")
	c.Assert(rev, Equals, 21)

	namespace, name, rev, err = formula.ParseId("local:mysql-cluster-21")
	c.Assert(err, IsNil)
	c.Assert(namespace, Equals, "local")
	c.Assert(name, Equals, "mysql-cluster")
	c.Assert(rev, Equals, 21)

	_, _, _, err = formula.ParseId("foo")
	c.Assert(err, Matches, `Missing formula namespace: "foo"`)

	_, _, _, err = formula.ParseId("local:foo-x")
	c.Assert(err, Matches, `Missing formula revision: "local:foo-x"`)
}
