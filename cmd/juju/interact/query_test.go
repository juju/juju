// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interact

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type Suite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(Suite{})

func (s *Suite) TestAnswer(c *tc.C) {
	scanner := bufio.NewScanner(strings.NewReader("hi!\n"))
	answer, err := QueryVerify("boo: ", scanner, io.Discard, io.Discard, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(answer, tc.Equals, "hi!")
}

func (s *Suite) TestVerify(c *tc.C) {
	scanner := bufio.NewScanner(strings.NewReader("hi!\nok!\n"))
	out := bytes.Buffer{}
	verify := func(s string) (ok bool, errmsg string, err error) {
		if s == "ok!" {
			return true, "", nil
		}
		return false, "No!", nil
	}
	answer, err := QueryVerify("boo: ", scanner, &out, &out, verify)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(answer, tc.Equals, "ok!")
	// in practice, "No!" will be on a separate line, since the cursor will get
	// moved down by the user hitting return for their answer, but the output
	// we generate doesn't do that itself.'
	expected := `
boo: No!

boo: 
`[1:]
	c.Assert(out.String(), tc.Equals, expected)
}

func (s *Suite) TestQueryMultiple(c *tc.C) {
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
	answer, err := QueryVerify("boo: ", scanner, io.Discard, io.Discard, verify)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(answer, tc.Equals, "ok!")

	answer, err = QueryVerify("name: ", scanner, io.Discard, io.Discard, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(answer, tc.Equals, "bob")
}

func (s *Suite) TestMatchOptions(c *tc.C) {
	f := MatchOptions([]string{"foo", "BAR"}, "nope")
	for _, s := range []string{"foo", "FOO", "BAR", "bar"} {
		ok, msg, err := f(s)
		c.Check(err, tc.ErrorIsNil)
		c.Check(msg, tc.Equals, "")
		c.Check(ok, tc.IsTrue)
	}
	ok, msg, err := f("baz")
	c.Check(err, tc.ErrorIsNil)
	c.Check(msg, tc.Equals, "nope")
	c.Check(ok, tc.IsFalse)
}

func (s *Suite) TestFindMatch(c *tc.C) {
	options := []string{"foo", "BAR"}
	m, ok := FindMatch("foo", options)
	c.Check(m, tc.Equals, "foo")
	c.Check(ok, tc.IsTrue)

	m, ok = FindMatch("FOO", options)
	c.Check(m, tc.Equals, "foo")
	c.Check(ok, tc.IsTrue)

	m, ok = FindMatch("bar", options)
	c.Check(m, tc.Equals, "BAR")
	c.Check(ok, tc.IsTrue)

	m, ok = FindMatch("BAR", options)
	c.Check(m, tc.Equals, "BAR")
	c.Check(ok, tc.IsTrue)

	_, ok = FindMatch("baz", options)
	c.Check(ok, tc.IsFalse)
}
