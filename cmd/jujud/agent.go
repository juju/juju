package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"regexp"
	"strings"
)

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// stateInfoValue implements gnuflag.Value on a state.Info.
type stateInfoValue state.Info

var validAddr = regexp.MustCompile("^.+:[0-9]+$")

// Set splits the comma-separated list of ZooKeeper addresses and stores
// onto v's Addrs. Addresses must include port numbers.
func (v *stateInfoValue) Set(value string) error {
	addrs := strings.Split(value, ",")
	for _, addr := range addrs {
		if !validAddr.MatchString(addr) {
			return fmt.Errorf("%q is not a valid zookeeper address", addr)
		}
	}
	v.Addrs = addrs
	return nil
}

// String returns the list of ZooKeeper addresses joined by commas.
func (v *stateInfoValue) String() string {
	if v.Addrs != nil {
		return strings.Join(v.Addrs, ",")
	}
	return ""
}

// stateInfoVar sets up a gnuflag flag analagously to FlagSet.*Var methods.
func stateInfoVar(fs *gnuflag.FlagSet, target *state.Info, name string, value []string, usage string) {
	target.Addrs = value
	fs.Var((*stateInfoValue)(target), name, usage)
}

// AgentConf handles command-line flags shared by all agents.
type AgentConf struct {
	JujuDir   string
	StateInfo state.Info
}

// addFlags injects common agent flags into f.
func (c *AgentConf) addFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.JujuDir, "juju-directory", environs.VarDir, "juju working directory")
	stateInfoVar(f, &c.StateInfo, "zookeeper-servers", nil, "zookeeper servers to connect to")
}

// checkArgs checks that required flags have been set and that args is empty.
func (c *AgentConf) checkArgs(args []string) error {
	if c.JujuDir == "" {
		return requiredError("juju-directory")
	}
	if c.StateInfo.Addrs == nil {
		return requiredError("zookeeper-servers")
	}
	return cmd.CheckEmpty(args)
}

type task interface {
	Stop() error
	Wait() error
}

// runTasks runs all the given tasks until any of them fails with an
// error.  It then stops all of them and returns that error.  If a value
// is received on the stop channel, the workers are stopped.
func runTasks(stop <-chan struct{}, tasks ...task) (err error) {
	done := make(chan error, len(tasks))
	for _, t := range tasks {
		t := t
		go func() {
			done <- t.Wait()
		}()
	}
waiting:
	for _ = range tasks {
		select {
		case err = <-done:
			if err != nil {
				break waiting
			}
		case <-stop:
			break waiting
		}
	}
	for _, t := range tasks {
		if terr := t.Stop(); terr != nil && err == nil {
			err = terr
		}
	}
	return
}
