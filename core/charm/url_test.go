// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v8"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type urlSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&urlSuite{})

func (s urlSuite) TestCharmURLSeriesToBase(c *gc.C) {
	tests := []struct {
		input  *charm.URL
		output *charm.URL
		err    string
	}{{
		input:  charm.MustParseURL("ch:foo"),
		output: charm.MustParseURL("ch:foo"),
	}, {
		input:  charm.MustParseURL("ch:amd64/foo"),
		output: charm.MustParseURL("ch:amd64/foo"),
	}, {
		input:  &charm.URL{Schema: "ch", Architecture: "amd64", Series: "focal", Name: "foo", Revision: -1},
		output: charm.MustParseURL("ch:amd64/ubuntu:20.04/foo"),
	}, {
		input:  &charm.URL{Schema: "ch", Architecture: "amd64", Series: "centos7", Name: "foo", Revision: 42},
		output: charm.MustParseURL("ch:amd64/centos:centos7/foo-42"),
	}, {
		input: &charm.URL{Schema: "ch", Architecture: "amd64", Series: "meshuggah", Name: "foo", Revision: 42},
		err:   `os name invalid: unknown OS for series: "meshuggah"`,
	}}
	for i, test := range tests {
		c.Logf("%d charm url %s", i, test.input)

		got, err := CharmURLSeriesToBase(test.input)
		if test.err != "" {
			c.Assert(test.err, gc.Equals, err.Error())
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		c.Assert(got, gc.DeepEquals, test.output)
	}
}
