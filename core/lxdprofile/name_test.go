package lxdprofile_test

import (
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type LXDProfileNameSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LXDProfileNameSuite{})

func (*LXDProfileNameSuite) TestProfileNames(c *gc.C) {
	testCases := []struct {
		input  []string
		output []string
	}{
		{
			input:  []string{},
			output: []string{},
		},
		{
			input:  []string{"default"},
			output: []string{},
		},
		{
			input: []string{
				lxdprofile.Name("foo", "bar", 1),
			},
			output: []string{
				lxdprofile.Name("foo", "bar", 1),
			},
		},
		{
			input: []string{
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("aaa", "bbb", 100),
			},
			output: []string{
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("aaa", "bbb", 100),
			},
		},
	}
	for _, tc := range testCases {
		c.Assert(lxdprofile.LXDProfileNames(tc.input), gc.DeepEquals, tc.output)
	}
}
