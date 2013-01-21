package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/deployer"
	"launchpad.net/tomb"
	"sync"
	"time"
)

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// AgentConf handles command-line flags shared by all agents.
type AgentConf struct {
	*agent.Conf
	dataDir string
}

// addFlags injects common agent flags into f.
func (c *AgentConf) addFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.dataDir, "data-dir", "/var/lib/juju", "directory for juju data")
}

func (c *AgentConf) checkArgs(args []string) error {
	if c.dataDir == "" {
		return requiredError("data-dir")
	}
	return cmd.CheckEmpty(args)
}

func (c *AgentConf) read(entityName string) error {
	var err error
	c.Conf, err = agent.ReadConf(c.dataDir, entityName)
	return err
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
			log.Printf("cmd/jujud: %s: %v", tasks[i], err)
		}
	}
	return chosen.err
}

func isUpgraded(err error) bool {
	_, ok := err.(*UpgradeReadyError)
	return ok
}

type Agent interface {
	Tomb() *tomb.Tomb
	RunOnce(st *state.State, entity AgentState) error
	Entity(st *state.State) (AgentState, error)
	EntityName() string
}

// runLoop repeatedly calls runOnce until it returns worker.ErrDead or
// an upgraded error, or a value is received on stop.
func runLoop(runOnce func() error, stop <-chan struct{}) error {
	log.Printf("cmd/jujud: agent starting")
	for {
		err := runOnce()
		if err == worker.ErrDead {
			log.Printf("cmd/jujud: entity is dead")
			return nil
		}
		if _, ok := err.(*UpgradeReadyError); ok {
			return err
		}
		if err == nil {
			log.Printf("cmd/jujud: agent died with no error")
		} else {
			log.Printf("cmd/jujud: %v", err)
		}
		if !isleep(retryDelay, stop) {
			return nil
		}
		log.Printf("cmd/jujud: rerunning agent")
	}
	panic("unreachable")
}

// isleep waits for the given duration or until it receives a value on
// stop.  It returns whether the full duration was slept without being
// stopped.
func isleep(d time.Duration, stop <-chan struct{}) bool {
	select {
	case <-stop:
		return false
	case <-time.After(d):
	}
	return true
}

// RunAgentLoop repeatedly connects to the state server
// and calls the agent's RunOnce method.
func RunAgentLoop(c *agent.Conf, a Agent) error {
	return runLoop(func() error {
		st, entity, err := openState(c, a)
		if err != nil {
			return err
		}
		defer st.Close()
		// TODO(rog) connect to API.
		return a.RunOnce(st, entity)
	}, a.Tomb().Dying())
}

// This mutex ensures that we can have two concurrent workers
// opening the same state without a problem with the
// password changing.
var openStateMutex sync.Mutex

func openState(c *agent.Conf, a Agent) (_ *state.State, _ AgentState, err error) {
	openStateMutex.Lock()
	defer openStateMutex.Unlock()

	st, newPassword, err0 := c.OpenState()
	if err0 != nil {
		return nil, nil, err0
	}
	defer func() {
		if err != nil {
			st.Close()
		}
	}()
	entity, err := a.Entity(st)
	if state.IsNotFound(err) || err == nil && entity.Life() == state.Dead {
		err = worker.ErrDead
	}
	if err != nil {
		return nil, nil, err
	}
	if newPassword != "" {
		// Ensure we do not lose changes made by another
		// worker by re-reading the configuration and changing
		// only the password.
		c1, err := agent.ReadConf(c.DataDir, c.EntityName())
		if err != nil {
			return nil, nil, err
		}
		c1.StateInfo.Password = newPassword
		if err := c1.Write(); err != nil {
			return nil, nil, err
		}
		if err := entity.SetMongoPassword(c1.StateInfo.Password); err != nil {
			return nil, nil, err
		}
		c.StateInfo.Password = newPassword
	}
	return st, entity, nil
}

// newDeployManager gives the tests the opportunity to create a deployer.Manager
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time we would
// otherwise be restricted to. When not testing, st is unused.
var newDeployManager = func(st *state.State, info *state.Info, dataDir string) deployer.Manager {
	// TODO: pick manager kind based on entity name? (once we have a
	// container manager for prinicpal units, that is; for now, there
	// is no distinction between principal and subordinate deployments)
	return deployer.NewSimpleManager(info, dataDir)
}

func newDeployer(st *state.State, w *state.UnitsWatcher, dataDir string) *deployer.Deployer {
	info := &state.Info{
		EntityName: w.EntityName(),
		Addrs:      st.Addrs(),
		CACert:     st.CACert(),
	}
	mgr := newDeployManager(st, info, dataDir)
	return deployer.NewDeployer(st, mgr, w)
}
