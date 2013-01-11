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
	"regexp"
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

func RunLoop(c *agent.Conf, a Agent) error {
	atomb := a.Tomb()
	for {
		log.Printf("cmd/jujud: agent starting")
		err := runOnce(c, a)
		if ug, ok := err.(*UpgradeReadyError); ok {
			if err = ug.ChangeAgentTools(); err == nil {
				// Return and let upstart deal with the restart.
				return ug
			}
		}
		if err == worker.ErrDead {
			log.Printf("cmd/jujud: agent is dead")
			return nil
		}
		if err == nil {
			log.Printf("cmd/jujud: workers died with no error")
		} else {
			log.Printf("cmd/jujud: %v", err)
		}
		select {
		case <-atomb.Dying():
			atomb.Kill(err)
		case <-time.After(retryDelay):
			log.Printf("cmd/jujud: rerunning machiner")
		}
		// Note: we don't want this at the head of the loop
		// because we want the agent to go through the body of
		// the loop at least once, even if it's been killed
		// before the start.
		if atomb.Err() != tomb.ErrStillAlive {
			break
		}
	}
	return atomb.Err()
}

func runOnce(c *agent.Conf, a Agent) error {
	st, passwordChanged, err := c.OpenState()
	if err != nil {
		return err
	}
	defer st.Close()
	entity, err := a.Entity(st)
	if state.IsNotFound(err) || err == nil && entity.Life() == state.Dead {
		return worker.ErrDead
	}
	if err != nil {
		return err
	}
	if passwordChanged {
		if err := c.Write(); err != nil {
			return err
		}
		if err := entity.SetPassword(c.StateInfo.Password); err != nil {
			return err
		}
	}
	return a.RunOnce(st, entity)
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
