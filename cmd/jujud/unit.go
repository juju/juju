package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"regexp"
	"strconv"
	"strings"
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	*agentConf
	UnitName string
}

func NewUnitAgent() *UnitAgent {
	return &UnitAgent{agentConf: &agentConf{name: "unit"}}
}

// InitFlagSet prepares a FlagSet.
func (a *UnitAgent) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&a.UnitName, "unit-name", a.UnitName, "name of the unit to run")
	a.agentConf.InitFlagSet(f)
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (a *UnitAgent) ParsePositional(args []string) error {
	if a.UnitName == "" {
		return requiredError("unit-name")
	}
	bad := fmt.Errorf("--unit-name option expects <service-name>/<non-negative integer>")
	split := strings.Split(a.UnitName, "/")
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
	return a.agentConf.ParsePositional(args)
}

// Run runs a unit agent.
func (a *UnitAgent) Run() error {
	// TODO connect to state once Open interface settles down
	// state, err := state.Open(a.zookeeper, a.sessionFile)
	// ...
	return fmt.Errorf("UnitAgent.Run not implemented")
}
