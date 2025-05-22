// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/url"
	"testing"

	"github.com/juju/gomaasapi/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
)

type environSuite struct {
	maasSuite
}

func TestEnvironSuite(t *testing.T) {
	tc.Run(t, &environSuite{})
}

func (*environSuite) TestConvertConstraints(c *tc.C) {
	for i, test := range []struct {
		cons     constraints.Value
		expected gomaasapi.AllocateMachineArgs
	}{{
		cons:     constraints.Value{Arch: stringp("arm")},
		expected: gomaasapi.AllocateMachineArgs{Architecture: "arm"},
	}, {
		cons:     constraints.Value{CpuCores: uint64p(4)},
		expected: gomaasapi.AllocateMachineArgs{MinCPUCount: 4},
	}, {
		cons:     constraints.Value{Mem: uint64p(1024)},
		expected: gomaasapi.AllocateMachineArgs{MinMemory: 1024},
	}, { // Spaces are converted to bindings and not_networks, but only in acquireNode
		cons:     constraints.Value{Spaces: stringslicep("foo", "bar", "^baz", "^oof")},
		expected: gomaasapi.AllocateMachineArgs{},
	}, {
		cons: constraints.Value{Tags: stringslicep("tag1", "tag2", "^tag3", "^tag4")},
		expected: gomaasapi.AllocateMachineArgs{
			Tags:    []string{"tag1", "tag2"},
			NotTags: []string{"tag3", "tag4"},
		},
	}, { // CpuPower is ignored.
		cons:     constraints.Value{CpuPower: uint64p(1024)},
		expected: gomaasapi.AllocateMachineArgs{},
	}, { // RootDisk is ignored.
		cons:     constraints.Value{RootDisk: uint64p(8192)},
		expected: gomaasapi.AllocateMachineArgs{},
	}, {
		cons: constraints.Value{Tags: stringslicep("foo", "bar")},
		expected: gomaasapi.AllocateMachineArgs{
			Tags: []string{"foo", "bar"},
		},
	}, {
		cons: constraints.Value{
			Arch:     stringp("arm"),
			CpuCores: uint64p(4),
			Mem:      uint64p(1024),
			CpuPower: uint64p(1024),
			RootDisk: uint64p(8192),
			Spaces:   stringslicep("foo", "^bar"),
			Tags:     stringslicep("^tag1", "tag2"),
		},
		expected: gomaasapi.AllocateMachineArgs{
			Architecture: "arm",
			MinCPUCount:  4,
			MinMemory:    1024,
			Tags:         []string{"tag2"},
			NotTags:      []string{"tag1"},
		},
	}} {
		c.Logf("test #%d: cons2=%s", i, test.cons.String())
		c.Check(convertConstraints(test.cons), tc.DeepEquals, test.expected)
	}
}

var nilStringSlice []string

func (*environSuite) TestConvertTagsToParams(c *tc.C) {
	for i, test := range []struct {
		tags     *[]string
		expected url.Values
	}{{
		tags:     nil,
		expected: url.Values{},
	}, {
		tags:     &nilStringSlice,
		expected: url.Values{},
	}, {
		tags:     &[]string{},
		expected: url.Values{},
	}, {
		tags:     stringslicep(""),
		expected: url.Values{},
	}, {
		tags: stringslicep("foo"),
		expected: url.Values{
			"tags": {"foo"},
		},
	}, {
		tags: stringslicep("^bar"),
		expected: url.Values{
			"not_tags": {"bar"},
		},
	}, {
		tags: stringslicep("foo", "^bar", "baz", "^oof"),
		expected: url.Values{
			"tags":     {"foo,baz"},
			"not_tags": {"bar,oof"},
		},
	}, {
		tags: stringslicep("", "^bar", "^", "^oof"),
		expected: url.Values{
			"not_tags": {"bar,oof"},
		},
	}, {
		tags: stringslicep("foo", "^", " b a z  ", "^^ ^"),
		expected: url.Values{
			"tags":     {"foo, b a z  "},
			"not_tags": {"^ ^"},
		},
	}, {
		tags: stringslicep("", "^bar", "  ", " ^ o of "),
		expected: url.Values{
			"tags":     {"  , ^ o of "},
			"not_tags": {"bar"},
		},
	}, {
		tags: stringslicep("foo", "foo", "^bar", "^bar"),
		expected: url.Values{
			"tags":     {"foo,foo"},
			"not_tags": {"bar,bar"},
		},
	}} {
		c.Logf("test #%d: tags=%v", i, test.tags)
		var vals = url.Values{}
		convertTagsToParams(vals, test.tags)
		c.Check(vals, tc.DeepEquals, test.expected)
	}
}

func uint64p(val uint64) *uint64 {
	return &val
}

func stringp(val string) *string {
	return &val
}

func stringslicep(values ...string) *[]string {
	return &values
}

func (suite *environSuite) TestParseDelimitedValues(c *tc.C) {
	for i, test := range []struct {
		about     string
		input     []string
		positives []string
		negatives []string
	}{{
		about:     "nil input",
		input:     nil,
		positives: []string{},
		negatives: []string{},
	}, {
		about:     "empty input",
		input:     []string{},
		positives: []string{},
		negatives: []string{},
	}, {
		about:     "values list with embedded whitespace",
		input:     []string{"   val1  ", " val2", " ^ not Val 3  ", "  ", " ", "^", "", "^ notVal4   "},
		positives: []string{"   val1  ", " val2", " ^ not Val 3  ", "  ", " "},
		negatives: []string{" notVal4   "},
	}, {
		about:     "only positives",
		input:     []string{"val1", "val2", "val3"},
		positives: []string{"val1", "val2", "val3"},
		negatives: []string{},
	}, {
		about:     "only negatives",
		input:     []string{"^val1", "^val2", "^val3"},
		positives: []string{},
		negatives: []string{"val1", "val2", "val3"},
	}, {
		about:     "multi-caret negatives",
		input:     []string{"^foo^", "^v^a^l2", "  ^^ ^", "^v^al3", "^^", "^"},
		positives: []string{"  ^^ ^"},
		negatives: []string{"foo^", "v^a^l2", "v^al3", "^"},
	}, {
		about:     "both positives and negatives",
		input:     []string{"^val1", "val2", "^val3", "val4"},
		positives: []string{"val2", "val4"},
		negatives: []string{"val1", "val3"},
	}, {
		about:     "single positive value",
		input:     []string{"val1"},
		positives: []string{"val1"},
		negatives: []string{},
	}, {
		about:     "single negative value",
		input:     []string{"^val1"},
		positives: []string{},
		negatives: []string{"val1"},
	}} {
		c.Logf("test %d: %s", i, test.about)
		positives, negatives := parseDelimitedValues(test.input)
		c.Check(positives, tc.DeepEquals, test.positives)
		c.Check(negatives, tc.DeepEquals, test.negatives)
	}
}
