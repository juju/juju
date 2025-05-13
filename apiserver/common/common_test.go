// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/internal/uuid"
)

type commonSuite struct{}

var _ = tc.Suite(&commonSuite{})

func errorAuth(context.Context) (common.AuthFunc, error) {
	return nil, fmt.Errorf("pow")
}

func fooAuth(context.Context) (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return tag == names.NewUserTag("foo")
	}, nil
}

func barAuth(context.Context) (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return tag == names.NewUserTag("bar")
	}, nil
}

func bazAuth(context.Context) (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return tag == names.NewUserTag("baz")
	}, nil
}

var authEitherTests = []struct {
	about  string
	a, b   func(context.Context) (common.AuthFunc, error)
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

func (s *commonSuite) TestAuthAnyCoversEither(c *tc.C) {
	for i, test := range authEitherTests {
		c.Logf("test %d: %s", i, test.about)
		authAny := common.AuthAny(test.a, test.b)
		any, err := authAny(context.Background())
		if test.err == "" {
			c.Assert(err, tc.ErrorIsNil)
			ok := any(test.tag)
			c.Assert(ok, tc.Equals, test.expect)
		} else {
			c.Assert(err, tc.ErrorMatches, test.err)
			c.Assert(any, tc.IsNil)
		}
	}
}

func (s *commonSuite) TestAuthAnyAlwaysFalseWithNoFuncs(c *tc.C) {
	getAuth := common.AuthAny()
	auth, err := getAuth(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(auth(names.NewUserTag("foo")), tc.IsFalse)
}

func (s *commonSuite) TestAuthAnyWith3(c *tc.C) {
	getAuth := common.AuthAny(fooAuth, barAuth, bazAuth)
	auth, err := getAuth(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(auth(names.NewUserTag("foo")), tc.IsTrue)
	c.Check(auth(names.NewUserTag("bar")), tc.IsTrue)
	c.Check(auth(names.NewUserTag("baz")), tc.IsTrue)
	c.Check(auth(names.NewUserTag("quux")), tc.IsFalse)
}

func u(unit string) names.Tag { return names.NewUnitTag(unit) }

func (s *commonSuite) TestAuthFuncForTagKind(c *tc.C) {
	// TODO(dimitern): This list of all supported tags and kinds needs
	// to live in juju/names.
	uuid, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	allTags := []names.Tag{
		nil, // invalid tag
		names.NewActionTag(uuid.String()),
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

		authFunc, err := getAuthFunc(context.Background())
		if allowedKind == "" {
			c.Check(err, tc.ErrorMatches, "tag kind cannot be empty")
			c.Check(authFunc, tc.IsNil)
			continue
		} else if !c.Check(err, tc.ErrorIsNil) {
			continue
		}

		for j, givenTag := range allTags {
			c.Logf("test #%d.%d: givenTag: %v", i, j, givenTag)

			var givenKind string
			if givenTag != nil {
				givenKind = givenTag.Kind()
			}
			if allowedKind == givenKind {
				c.Check(authFunc(givenTag), tc.IsTrue)
			} else {
				c.Check(authFunc(givenTag), tc.IsFalse)
			}
		}
	}
}
