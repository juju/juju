// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interact

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type Suite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(Suite{})

func (Suite) TestAnswer(c *gc.C) {
	scanner := bufio.NewScanner(strings.NewReader("hi!\n"))
	answer, err := QueryVerify("boo: ", scanner, ioutil.Discard, ioutil.Discard, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(answer, gc.Equals, "hi!")
}

func (Suite) TestVerify(c *gc.C) {
	scanner := bufio.NewScanner(strings.NewReader("hi!\nok!\n"))
	out := bytes.Buffer{}
	verify := func(s string) (ok bool, errmsg string, err error) {
		if s == "ok!" {
			return true, "", nil
		}
		return false, "No!", nil
	}
	answer, err := QueryVerify("boo: ", scanner, &out, &out, verify)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(answer, gc.Equals, "ok!")
	// in practice, "No!" will be on a separate line, since the cursor will get
	// moved down by the user hitting return for their answer, but the output
	// we generate doesn't do that itself.'
	expected := `
boo: No!

boo: 
`[1:]
	c.Assert(out.String(), gc.Equals, expected)
}

func (Suite) TestQueryMultiple(c *gc.C) {
	scanner := bufio.NewScanner(strings.NewReader(`
hi!
ok!
bob
`[1:]))
	verify := func(s string) (ok bool, errmsg string, err error) {
		if s == "ok!" {
			return true, "", nil
		}
		return false, "No!", nil
	}
	answer, err := QueryVerify("boo: ", scanner, ioutil.Discard, ioutil.Discard, verify)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(answer, gc.Equals, "ok!")

	answer, err = QueryVerify("name: ", scanner, ioutil.Discard, ioutil.Discard, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(answer, gc.Equals, "bob")
}

func (Suite) TestMatchOptions(c *gc.C) {
	f := MatchOptions([]string{"foo", "BAR"}, "nope")
	for _, s := range []string{"foo", "FOO", "BAR", "bar"} {
		ok, msg, err := f(s)
		c.Check(err, jc.ErrorIsNil)
		c.Check(msg, gc.Equals, "")
		c.Check(ok, jc.IsTrue)
	}
	ok, msg, err := f("baz")
	c.Check(err, jc.ErrorIsNil)
	c.Check(msg, gc.Equals, "nope")
	c.Check(ok, jc.IsFalse)
}

func (Suite) TestFindMatch(c *gc.C) {
	options := []string{"foo", "BAR"}
	m, ok := FindMatch("foo", options)
	c.Check(m, gc.Equals, "foo")
	c.Check(ok, jc.IsTrue)

	m, ok = FindMatch("FOO", options)
	c.Check(m, gc.Equals, "foo")
	c.Check(ok, jc.IsTrue)

	m, ok = FindMatch("bar", options)
	c.Check(m, gc.Equals, "BAR")
	c.Check(ok, jc.IsTrue)

	m, ok = FindMatch("BAR", options)
	c.Check(m, gc.Equals, "BAR")
	c.Check(ok, jc.IsTrue)

	m, ok = FindMatch("baz", options)
	c.Check(ok, jc.IsFalse)
}
