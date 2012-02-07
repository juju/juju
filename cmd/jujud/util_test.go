package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
)

func ParseAgentFlags(c *C, f main.AgentFlags, args []string) error {
	common := []string{
		"--zookeeper-servers", "zk",
		"--session-file", "sf",
		"--juju-directory", "jd",
	}
	ac := main.NewAgentCommand(f)
	return cmd.Parse(ac, true, append(common, args...))
}
