// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/names"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type commonSuite struct{}

var _ = gc.Suite(&commonSuite{})

func errorAuth() (common.AuthFunc, error) {
	return nil, fmt.Errorf("pow")
}

func fooAuth() (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return tag == names.NewUserTag("foo")
	}, nil
}

func barAuth() (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return tag == names.NewUserTag("bar")
	}, nil
}

var authEitherTests = []struct {
	about  string
	a, b   func() (common.AuthFunc, error)
	tag    names.Tag
	expect bool
	err    string
}{{
	about: "a returns an error",
	a:     errorAuth,
	b:     fooAuth,
	err:   "pow",
}, {
	about: "b returns an error",
	a:     fooAuth,
	b:     errorAuth,
	err:   "pow",
}, {
	about: "both a and b return an error",
	a:     errorAuth,
	b:     errorAuth,
	err:   "pow",
}, {
	about:  "tag foo - a returns true",
	a:      fooAuth,
	b:      barAuth,
	tag:    names.NewUserTag("foo"),
	expect: true,
}, {
	about:  "tag foo - b returns true",
	a:      barAuth,
	b:      fooAuth,
	tag:    names.NewUserTag("foo"),
	expect: true,
}, {
	about:  "tag bar - b returns true",
	a:      fooAuth,
	b:      barAuth,
	tag:    names.NewUserTag("bar"),
	expect: true,
}, {
	about:  "tag foo - both return true",
	a:      fooAuth,
	b:      fooAuth,
	tag:    names.NewUserTag("foo"),
	expect: true,
}, {
	about:  "tag baz - both return false",
	a:      fooAuth,
	b:      barAuth,
	tag:    names.NewUserTag("baz"),
	expect: false,
}, {
	about:  "tag quxx - both return false",
	a:      fooAuth,
	b:      barAuth,
	tag:    names.NewServiceTag("quxx"),
	expect: false,
}}

func (s *commonSuite) TestAuthEither(c *gc.C) {
	for i, test := range authEitherTests {
		c.Logf("test %d: %s", i, test.about)
		authEither := common.AuthEither(test.a, test.b)
		either, err := authEither()
		if test.err == "" {
			c.Assert(err, gc.IsNil)
			ok := either(test.tag)
			c.Assert(ok, gc.Equals, test.expect)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
			c.Assert(either, gc.IsNil)
		}
	}
}

func u(unit string) names.Tag    { return names.NewUnitTag(unit) }
func m(machine string) names.Tag { return names.NewMachineTag(machine) }
