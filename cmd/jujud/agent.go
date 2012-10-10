package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
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

// Set splits the comma-separated list of state server addresses and stores
// onto v's Addrs. Addresses must include port numbers.
func (v *stateInfoValue) Set(value string) error {
	addrs := strings.Split(value, ",")
	for _, addr := range addrs {
		if !validAddr.MatchString(addr) {
			return fmt.Errorf("%q is not a valid state server address", addr)
		}
	}
	v.Addrs = addrs
	return nil
}

// String returns the list of server addresses joined by commas.
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
	accept          agentFlags
	DataDir         string
	StateInfo       state.Info
	InitialPassword string
}

type agentFlags int

const (
	flagStateInfo agentFlags = 1 << iota
	flagInitialPassword
	flagDataDir

	flagAll agentFlags = ^0
)

// addFlags injects common agent flags into f.
func (c *AgentConf) addFlags(f *gnuflag.FlagSet, accept agentFlags) {
	if accept&flagDataDir != 0 {
		f.StringVar(&c.DataDir, "data-dir", "/var/lib/juju", "directory for juju data")
	}
	if accept&flagStateInfo != 0 {
		stateInfoVar(f, &c.StateInfo, "state-servers", nil, "state servers to connect to")
	}
	if accept&flagInitialPassword != 0 {
		f.StringVar(&c.InitialPassword, "initial-password", "", "initial password for state")
	}
	c.accept = accept
}

// checkArgs checks that required flags have been set and that args is empty.
func (c *AgentConf) checkArgs(args []string) error {
	if c.accept&flagDataDir != 0 && c.DataDir == "" {
		return requiredError("data-dir")
	}
	if c.accept&flagStateInfo != 0 && c.StateInfo.Addrs == nil {
		return requiredError("state-servers")
	}
	return cmd.CheckEmpty(args)
}

type task interface {
	Stop() error
	Wait() error
	String() string
}

// runTasks runs all the given tasks until any of them fails with an
// error.  It then stops all of them and returns that error.  If a value
// is received on the stop channel, the workers are stopped.
// The task values should be comparable.
func runTasks(stop <-chan struct{}, tasks ...task) (err error) {
	type errInfo struct {
		index int
		err   error
	}
	done := make(chan errInfo, len(tasks))
	for i, t := range tasks {
		i, t := i, t
		go func() {
			done <- errInfo{i, t.Wait()}
		}()
	}
	chosen := errInfo{index: -1}
waiting:
	for _ = range tasks {
		select {
		case info := <-done:
			if info.err != nil {
				chosen = info
				break waiting
			}
		case <-stop:
			break waiting
		}
	}
	// Stop all the tasks. If we've been upgraded,
	// that error taks precedence over other errors, because
	// that's the only way we can escape bad code.
	for i, t := range tasks {
		if err := t.Stop(); isUpgraded(err) || err != nil && chosen.err == nil {
			chosen = errInfo{i, err}
		}
	}
	// Log any errors that we're discarding.
	for i, t := range tasks {
		if i == chosen.index {
			continue
		}
		if err := t.Wait(); err != nil {
			log.Printf("%s: %v", tasks[i], err)
		}
	}
	return chosen.err
}

func isUpgraded(err error) bool {
	_, ok := err.(*UpgradeReadyError)
	return ok
}

// openState tries to open the state with the given entity name
// and configuration information.
func openState(entityName string, conf *AgentConf) (*state.State, error) {
	// TODO read password from file and try to use that
	info := conf.StateInfo
	// TODO remove this test when passwords are correctly set
	// up before starting agents.
	if conf.InitialPassword != "" {
		info.EntityName = entityName
		info.Password = conf.InitialPassword
	}
	return state.Open(&info)
}
