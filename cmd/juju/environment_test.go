package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"strings"
)

type GetEnvironmentSuite struct {
	repoSuite
}

var _ = Suite(&GetEnvironmentSuite{})

var singleValueTests = []struct {
	key    string
	output string
	err    string
}{
	{
		key:    "type",
		output: "dummy",
	}, {
		key:    "name",
		output: "dummyenv",
	}, {
		key:    "authorized-keys",
		output: "i-am-a-key",
	}, {
		key: "unknown",
		err: `Environment key "unknown" not found in "dummyenv" environment.`,
	},
}

func (s *GetEnvironmentSuite) TestSingleValue(c *C) {

	for _, t := range singleValueTests {
		context, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{t.key})
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			output := strings.TrimSpace(testing.Stdout(context))
			c.Assert(output, Equals, t.output)
		}
	}
}
