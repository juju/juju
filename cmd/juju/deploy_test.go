package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type DeploySuite struct {
	testing.RepoSuite
}

var _ = Suite(&DeploySuite{})

func runDeploy(c *C, args ...string) error {
	_, err := coretesting.RunCommand(c, &DeployCommand{}, args)
	return err
}

var initErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: nil,
		err:  `no charm specified`,
	}, {
		args: []string{"craz~ness"},
		err:  `invalid charm name "craz~ness"`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid service name "burble-1"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "0"},
		err:  `must deploy at least one unit`,
	}, {
		args: []string{"craziness", "burble1", "--constraints", "gibber=plop"},
		err:  `invalid value "gibber=plop" for flag --constraints: unknown constraint "gibber"`,
	},
}

func (s *DeploySuite) TestInitErrors(c *C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(&DeployCommand{}, t.args)
		c.Assert(err, ErrorMatches, t.err)
	}
}
