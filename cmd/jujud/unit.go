package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
	"regexp"
	"strconv"
	"strings"
)

type UnitCommand struct {
	conf     *AgentConf
	UnitName string
}

func NewUnitCommand() *UnitCommand {
	return &UnitCommand{conf: NewAgentConf()}
}

// Info returns a decription of the command.
func (c *UnitCommand) Info() *cmd.Info {
	return &cmd.Info{"unit", "[options]", "run a juju unit agent", "", true}
}

// InitFlagSet prepares a FlagSet.
func (c *UnitCommand) InitFlagSet(f *gnuflag.FlagSet) {
	c.conf.InitFlagSet(f)
	f.StringVar(&c.UnitName, "unit-name", c.UnitName, "name of the unit to run")
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (c *UnitCommand) ParsePositional(args []string) error {
	if err := c.conf.Validate(); err != nil {
		return err
	}
	if c.UnitName == "" {
		return requiredError("unit-name")
	}

	bad := fmt.Errorf("--unit-name option expects <service-name>/<non-negative integer>")
	split := strings.Split(c.UnitName, "/")
	if len(split) != 2 {
		return bad
	}
	validService := regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
	if !validService.MatchString(split[0]) {
		return bad
	}
	if _, err := strconv.ParseUint(split[1], 10, 0); err != nil {
		return bad
	}
	return cmd.CheckEmpty(args)
}

// Run runs a unit agent.
func (c *UnitCommand) Run() error {
	return StartAgent(c.conf, &UnitAgent{Name: c.UnitName})
}

// UnitAgent is responsible for managing a single service unit.
type UnitAgent struct {
	Name string
}

// Run runs the agent.
func (a *UnitAgent) Run(state *state.State, jujuDir string) error {
	return fmt.Errorf("UnitAgent.Run not implemented")
}
