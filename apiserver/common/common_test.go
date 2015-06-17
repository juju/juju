// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

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
			c.Assert(err, jc.ErrorIsNil)
			ok := either(test.tag)
			c.Assert(ok, gc.Equals, test.expect)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
			c.Assert(either, gc.IsNil)
		}
	}
}

func u(unit string) names.Tag             { return names.NewUnitTag(unit) }
func serviceTag(service string) names.Tag { return names.NewServiceTag(service) }
func m(machine string) names.Tag          { return names.NewMachineTag(machine) }

func (s *commonSuite) TestAuthFuncForTagKind(c *gc.C) {
	// TODO(dimitern): This list of all supported tags and kinds needs
	// to live in juju/names.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	allTags := []names.Tag{
		nil, // invalid tag
		names.NewActionTag(uuid.String()),
		names.NewCharmTag("cs:precise/missing"),
		names.NewEnvironTag(uuid.String()),
		names.NewFilesystemTag("20/20"),
		names.NewLocalUserTag("user"),
		names.NewMachineTag("42"),
		names.NewNetworkTag("public"),
		names.NewRelationTag("wordpress:mysql mysql:db"),
		names.NewServiceTag("wordpress"),
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
