// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	coretesting "github.com/juju/juju/testing"
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

func bazAuth() (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return tag == names.NewUserTag("baz")
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
	tag:    names.NewApplicationTag("quxx"),
	expect: false,
}}

func (s *commonSuite) TestAuthAnyCoversEither(c *gc.C) {
	for i, test := range authEitherTests {
		c.Logf("test %d: %s", i, test.about)
		authAny := common.AuthAny(test.a, test.b)
		any, err := authAny()
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			ok := any(test.tag)
			c.Assert(ok, gc.Equals, test.expect)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
			c.Assert(any, gc.IsNil)
		}
	}
}

func (s *commonSuite) TestAuthAnyAlwaysFalseWithNoFuncs(c *gc.C) {
	getAuth := common.AuthAny()
	auth, err := getAuth()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(auth(names.NewUserTag("foo")), jc.IsFalse)
}

func (s *commonSuite) TestAuthAnyWith3(c *gc.C) {
	getAuth := common.AuthAny(fooAuth, barAuth, bazAuth)
	auth, err := getAuth()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(auth(names.NewUserTag("foo")), jc.IsTrue)
	c.Check(auth(names.NewUserTag("bar")), jc.IsTrue)
	c.Check(auth(names.NewUserTag("baz")), jc.IsTrue)
	c.Check(auth(names.NewUserTag("quux")), jc.IsFalse)
}

func u(unit string) names.Tag { return names.NewUnitTag(unit) }

func (s *commonSuite) TestAuthFuncForTagKind(c *gc.C) {
	// TODO(dimitern): This list of all supported tags and kinds needs
	// to live in juju/names.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	allTags := []names.Tag{
		nil, // invalid tag
		names.NewActionTag(uuid.String()),
		names.NewCharmTag("cs:precise/missing"),
		names.NewModelTag(uuid.String()),
		names.NewFilesystemTag("20/20"),
		names.NewLocalUserTag("user"),
		names.NewMachineTag("42"),
		names.NewRelationTag("wordpress:mysql mysql:db"),
		names.NewApplicationTag("wordpress"),
		names.NewSpaceTag("apps"),
		names.NewStorageTag("foo/42"),
		names.NewUnitTag("wordpress/5"),
		names.NewUserTag("joe"),
		names.NewVolumeTag("80/20"),
	}
	for i, allowedTag := range allTags {
		c.Logf("test #%d: allowedTag: %v", i, allowedTag)

		var allowedKind string
		if allowedTag != nil {
			allowedKind = allowedTag.Kind()
		}
		getAuthFunc := common.AuthFuncForTagKind(allowedKind)

		authFunc, err := getAuthFunc()
		if allowedKind == "" {
			c.Check(err, gc.ErrorMatches, "tag kind cannot be empty")
			c.Check(authFunc, gc.IsNil)
			continue
		} else if !c.Check(err, jc.ErrorIsNil) {
			continue
		}

		for j, givenTag := range allTags {
			c.Logf("test #%d.%d: givenTag: %v", i, j, givenTag)

			var givenKind string
			if givenTag != nil {
				givenKind = givenTag.Kind()
			}
			if allowedKind == givenKind {
				c.Check(authFunc(givenTag), jc.IsTrue)
			} else {
				c.Check(authFunc(givenTag), jc.IsFalse)
			}
		}
	}
}
