// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/fslock"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	apiagent "github.com/juju/juju/state/api/agent"
	apideployer "github.com/juju/juju/state/api/deployer"
	"github.com/juju/juju/state/api/params"
	apirsyslog "github.com/juju/juju/state/api/rsyslog"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/upgrader"
)

var apiOpen = api.Open
var dataDir = paths.MustSucceed(paths.DataDir(version.Current.Series))

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// AgentConf handles command-line flags shared by all agents.
type AgentConf struct {
	dataDir string
	mu      sync.Mutex
	_config agent.ConfigSetterWriter
}

// AddFlags injects common agent flags into f.
func (c *AgentConf) AddFlags(f *gnuflag.FlagSet) {
	// TODO(dimitern) 2014-02-19 bug 1282025
	// We need to pass a config location here instead and
	// use it to locate the conf and the infer the data-dir
	// from there instead of passing it like that.
	f.StringVar(&c.dataDir, "data-dir", dataDir, "directory for juju data")
}

func (c *AgentConf) CheckArgs(args []string) error {
	if c.dataDir == "" {
		return requiredError("data-dir")
	}
	return cmd.CheckEmpty(args)
}

func (c *AgentConf) ReadConfig(tag string) error {
	t, err := names.ParseTag(tag)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	conf, err := agent.ReadConfig(agent.ConfigPath(c.dataDir, t))
	if err != nil {
		return err
	}
	c._config = conf
	return nil
}

func (ch *AgentConf) ChangeConfig(change func(c agent.ConfigSetter)) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	change(ch._config)
	return ch._config.Write()
}

func (ch *AgentConf) CurrentConfig() agent.Config {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch._config.Clone()
}

// SetAPIHostPorts satisfies worker/apiaddressupdater/APIAddressSetter.
func (a *AgentConf) SetAPIHostPorts(servers [][]network.HostPort) error {
	return a.ChangeConfig(func(c agent.ConfigSetter) {
		c.SetAPIHostPorts(servers)
	})
}

func importance(err error) int {
	switch {
	case err == nil:
		return 0
	default:
		return 1
	case isUpgraded(err):
		return 2
	case err == worker.ErrTerminateAgent:
		return 3
	}
}

// moreImportant returns whether err0 is
// more important than err1 - that is, whether
// we should act on err0 in preference to err1.
func moreImportant(err0, err1 error) bool {
	return importance(err0) > importance(err1)
}

func isUpgraded(err error) bool {
	_, ok := err.(*upgrader.UpgradeReadyError)
	return ok
}

type Agent interface {
	Tag() names.Tag
	ChangeConfig(func(agent.ConfigSetter)) error
}

// The AgentState interface is implemented by state types
// that represent running agents.
type AgentState interface {
	// SetAgentVersion sets the tools version that the agent is
	// currently running.
	SetAgentVersion(v version.Binary) error
	Tag() string
	SetMongoPassword(password string) error
	Life() state.Life
}

type fatalError struct {
	Err string
}

func (e *fatalError) Error() string {
	return e.Err
}

func isFatal(err error) bool {
	if err == worker.ErrTerminateAgent {
		return true
	}
	if isUpgraded(err) {
		return true
	}
	_, ok := err.(*fatalError)
	return ok
}

type pinger interface {
	Ping() error
}

// connectionIsFatal returns a function suitable for passing
// as the isFatal argument to worker.NewRunner,
// that diagnoses an error as fatal if the connection
// has failed or if the error is otherwise fatal.
func connectionIsFatal(conn pinger) func(err error) bool {
	return func(err error) bool {
		if isFatal(err) {
			return true
		}
		if err := conn.Ping(); err != nil {
			logger.Infof("error pinging %T: %v", conn, err)
			return true
		}
		return false
	}
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

type apiOpener interface {
	OpenAPI(api.DialOpts) (*api.State, string, error)
}

type configChanger func(c *agent.Config)

// openAPIState opens the API using the given information, and
// returns the opened state and the api entity with
// the given tag. The given changeConfig function is
// called if the password changes to set the password.
func openAPIState(agentConfig agent.Config, a Agent) (_ *api.State, _ *apiagent.Entity, resultErr error) {
	// We let the API dial fail immediately because the
	// runner's loop outside the caller of openAPIState will
	// keep on retrying. If we block for ages here,
	// then the worker that's calling this cannot
	// be interrupted.
	info := agentConfig.APIInfo()
	st, err := apiOpen(info, api.DialOpts{})
	usedOldPassword := false
	if params.IsCodeUnauthorized(err) {
		// We've perhaps used the wrong password, so
		// try again with the fallback password.
		info := *info
		info.Password = agentConfig.OldPassword()
		usedOldPassword = true
		st, err = apiOpen(&info, api.DialOpts{})
	}
	if err != nil {
		if params.IsCodeNotProvisioned(err) {
			return nil, nil, worker.ErrTerminateAgent
		}
		if params.IsCodeUnauthorized(err) {
			return nil, nil, worker.ErrTerminateAgent
		}
		return nil, nil, err
	}
	defer func() {
		if resultErr != nil {
			st.Close()
		}
	}()
	entity, err := st.Agent().Entity(a.Tag())
	if err == nil && entity.Life() == params.Dead {
		return nil, nil, worker.ErrTerminateAgent
	}
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			return nil, nil, worker.ErrTerminateAgent
		}
		return nil, nil, err
	}
	if usedOldPassword {
		// We succeeded in connecting with the fallback
		// password, so we need to create a new password
		// for the future.

		newPassword, err := utils.RandomPassword()
		if err != nil {
			return nil, nil, err
		}
		// Change the configuration *before* setting the entity
		// password, so that we avoid the possibility that
		// we might successfully change the entity's
		// password but fail to write the configuration,
		// thus locking us out completely.
		if err := a.ChangeConfig(func(c agent.ConfigSetter) {
			c.SetPassword(newPassword)
			c.SetOldPassword(info.Password)
		}); err != nil {
			return nil, nil, err
		}
		if err := entity.SetPassword(newPassword); err != nil {
			return nil, nil, err
		}
	}

	return st, entity, nil
}

// agentDone processes the error returned by
// an exiting agent.
func agentDone(err error) error {
	if err == worker.ErrTerminateAgent {
		err = nil
	}
	if ug, ok := err.(*upgrader.UpgradeReadyError); ok {
		if err := ug.ChangeAgentTools(); err != nil {
			// Return and let upstart deal with the restart.
			return errors.LoggedErrorf(logger, "cannot change agent tools: %v", err)
		}
	}
	return err
}

type closeWorker struct {
	worker worker.Worker
	closer io.Closer
}

// newCloseWorker returns a task that wraps the given task,
// closing the given closer when it finishes.
func newCloseWorker(worker worker.Worker, closer io.Closer) worker.Worker {
	return &closeWorker{
		worker: worker,
		closer: closer,
	}
}

func (c *closeWorker) Kill() {
	c.worker.Kill()
}

func (c *closeWorker) Wait() error {
	err := c.worker.Wait()
	if err := c.closer.Close(); err != nil {
		logger.Errorf("closeWorker: close error: %v", err)
	}
	return err
}

// newDeployContext gives the tests the opportunity to create a deployer.Context
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time to which we would
// otherwise be restricted.
var newDeployContext = func(st *apideployer.State, agentConfig agent.Config) deployer.Context {
	return deployer.NewSimpleContext(agentConfig, st)
}

// newRsyslogConfigWorker creates and returns a new RsyslogConfigWorker
// based on the specified configuration parameters.
var newRsyslogConfigWorker = func(st *apirsyslog.State, agentConfig agent.Config, mode rsyslog.RsyslogMode) (worker.Worker, error) {
	tag := agentConfig.Tag()
	namespace := agentConfig.Value(agent.Namespace)
	addrs, err := agentConfig.APIAddresses()
	if err != nil {
		return nil, err
	}
	return rsyslog.NewRsyslogConfigWorker(st, mode, tag, namespace, addrs)
}

// hookExecutionLock returns an *fslock.Lock suitable for use as a unit
// hook execution lock. Other workers may also use this lock if they
// require isolation from hook execution.
func hookExecutionLock(dataDir string) (*fslock.Lock, error) {
	lockDir := filepath.Join(dataDir, "locks")
	return fslock.NewLock(lockDir, "uniter-hook-execution")
}
