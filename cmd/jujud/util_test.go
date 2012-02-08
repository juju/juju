package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
)

// ParseAgentFlags is a utility function used by individual agent tests.
func ParseAgentFlags(c *C, f main.AgentFlags, args []string) error {
	common := []string{
		"--zookeeper-servers", "zk",
		"--session-file", "sf",
		"--juju-directory", "jd",
	}
	ac := main.NewAgentCommand(f)
	return cmd.Parse(ac, append(common, args...))
}
