// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type tagSuite struct{}

var _ = gc.Suite(&tagSuite{})

var tagKindTests = []struct {
	tag  string
	kind string
	err  string
}{
	{tag: "unit-wordpress-42", kind: names.UnitTagKind},
	{tag: "machine-42", kind: names.MachineTagKind},
	{tag: "service-foo", kind: names.ServiceTagKind},
	{tag: "environment-42", kind: names.EnvironTagKind},
	{tag: "user-admin", kind: names.UserTagKind},
	{tag: "relation-service1.rel1#other-svc.other-rel2", kind: names.RelationTagKind},
	{tag: "relation-service.peerRelation", kind: names.RelationTagKind},
	{tag: "foo", err: `"foo" is not a valid tag`},
	{tag: "unit", err: `"unit" is not a valid tag`},
}

func (*tagSuite) TestTagKind(c *gc.C) {
	for i, test := range tagKindTests {
		c.Logf("test %d: %q -> %q", i, test.tag, test.kind)
		kind, err := names.TagKind(test.tag)
		if test.err == "" {
			c.Assert(test.kind, gc.Equals, kind)
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(kind, gc.Equals, "")
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

var parseTagTests = []struct {
	tag        string
	expectKind string
	resultId   string
	resultErr  string
}{{
	tag:        "machine-10",
	expectKind: names.MachineTagKind,
	resultId:   "10",
}, {
	tag:        "machine-10-lxc-1",
	expectKind: names.MachineTagKind,
	resultId:   "10/lxc/1",
}, {
	tag:        "foo",
	expectKind: names.MachineTagKind,
	resultErr:  `"foo" is not a valid machine tag`,
}, {
	tag:        "machine-#",
	expectKind: names.MachineTagKind,
	resultErr:  `"machine-#" is not a valid machine tag`,
}, {
	tag:        "unit-wordpress-0",
	expectKind: names.UnitTagKind,
	resultId:   "wordpress/0",
}, {
	tag:        "unit-rabbitmq-server-0",
	expectKind: names.UnitTagKind,
	resultId:   "rabbitmq-server/0",
}, {
	tag:        "foo",
	expectKind: names.UnitTagKind,
	resultErr:  `"foo" is not a valid unit tag`,
}, {
	tag:        "unit-#",
	expectKind: names.UnitTagKind,
	resultErr:  `"unit-#" is not a valid unit tag`,
}, {
	tag:        "service-wordpress",
	expectKind: names.ServiceTagKind,
	resultId:   "wordpress",
}, {
	tag:        "service-#",
	expectKind: names.ServiceTagKind,
	resultErr:  `"service-#" is not a valid service tag`,
}, {
	tag:        "unit-wordpress-0",
	expectKind: "machine",
	resultErr:  `"unit-wordpress-0" is not a valid machine tag`,
}, {
	tag:        "environment-foo",
	expectKind: names.EnvironTagKind,
	resultId:   "foo",
}, {
	tag:        "relation-my-svc1.myrel1#other-svc.other-rel2",
	expectKind: names.RelationTagKind,
	resultId:   "my-svc1:myrel1 other-svc:other-rel2",
}, {
	tag:        "relation-riak.ring",
	expectKind: names.RelationTagKind,
	resultId:   "riak:ring",
}, {
	tag:        "environment-/",
	expectKind: names.EnvironTagKind,
	resultErr:  `"environment-/" is not a valid environment tag`,
}, {
	tag:        "user-foo",
	expectKind: names.UserTagKind,
	resultId:   "foo",
}, {
	tag:        "user-/",
	expectKind: names.UserTagKind,
	resultErr:  `"user-/" is not a valid user tag`,
}, {
	tag:        "foo",
	expectKind: "",
	resultErr:  `"foo" is not a valid tag`,
}}

var makeTag = map[string]func(id string) string{
	names.MachineTagKind:  names.MachineTag,
	names.UnitTagKind:     names.UnitTag,
	names.ServiceTagKind:  names.ServiceTag,
	names.RelationTagKind: names.RelationTag,
	// TODO(rog) environment and user, when they have Tag functions.
}

func (*tagSuite) TestParseTag(c *gc.C) {
	for i, test := range parseTagTests {
		c.Logf("test %d: %q expectKind %q", i, test.tag, test.expectKind)
		kind, id, err := names.ParseTag(test.tag, test.expectKind)
		if test.resultErr != "" {
			c.Assert(err, gc.ErrorMatches, test.resultErr)
			c.Assert(kind, gc.Equals, "")
			c.Assert(id, gc.Equals, "")

			// If the tag has a valid kind which matches the
			// expected kind, test that using an empty
			// expectKind does not change the error message.
			if tagKind, err := names.TagKind(test.tag); err == nil && tagKind == test.expectKind {
				kind, id, err := names.ParseTag(test.tag, "")
				c.Assert(err, gc.ErrorMatches, test.resultErr)
				c.Assert(kind, gc.Equals, "")
				c.Assert(id, gc.Equals, "")
			}
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(id, gc.Equals, test.resultId)
			if test.expectKind != "" {
				c.Assert(kind, gc.Equals, test.expectKind)
			} else {
				expectKind, err := names.TagKind(test.tag)
				c.Assert(err, gc.IsNil)
				c.Assert(kind, gc.Equals, expectKind)
			}
			// Check that it's reversible.
			if f := makeTag[kind]; f != nil {
				reversed := f(id)
				c.Assert(reversed, gc.Equals, test.tag)
			}
			// Check that it parses ok without an expectKind.
			kind1, id1, err1 := names.ParseTag(test.tag, "")
			c.Assert(err1, gc.IsNil)
			c.Assert(kind1, gc.Equals, test.expectKind)
			c.Assert(id1, gc.Equals, id)
		}
	}
}
