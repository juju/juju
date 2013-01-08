package main

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

var validAddr = regexp.MustCompile("^.+:[0-9]+$")

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

func RunLoop(c *AgentConf, a Agent) error {
	atomb := a.Tomb()
	for atomb.Err() == tomb.ErrStillAlive {
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
	}
	return atomb.Err()
}

func runOnce(c *AgentConf, a Agent) error {
	st, password, err := openState(a.EntityName(), c)
	defer st.Close()
	entity, err := a.Entity(st)
	if state.IsNotFound(err) || err == nil && entity.Life() == state.Dead {
		return worker.ErrDead
	}
	if err != nil {
		return err
	}
	if password != "" {
		if err := entity.SetPassword(password); err != nil {
			return err
		}
	}
	return a.RunOnce(st, entity)
}

// openState tries to open the state with the given entity name
// and configuration information. If the returned password
// is non-empty, the caller should set the entity's password
// accordingly.
func openState(entityName string, conf *AgentConf) (st *state.State, password string, err error) {
	info := conf.StateInfo
	if info.Password != "" {
		st, err := state.Open(&info)
		if err == nil {
			return st, "", nil
		}
		if err != state.ErrUnauthorized {
			return nil, "", err
		}
		// Access isn't authorized even though the password was
		// saved.  This can happen if we crash after saving the
		// password but before changing it, so we'll try again
		// with the initial password.
	}
	info.Password = conf.InitialPassword
	st, err = state.Open(&info)
	if err != nil {
		return nil, "", err
	}
	// We've succeeded in connecting with the initial password, so
	// we can now change it to something more private.
	password, err = trivial.RandomPassword()
	if err != nil {
		st.Close()
		return nil, "", err
	}
	if err := ioutil.WriteFile(pwfile, []byte(password), 0600); err != nil {
		st.Close()
		return nil, "", fmt.Errorf("cannot save password: %v", err)
	}
	return st, password, nil
}
