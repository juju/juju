// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
agent contains jujud's machine agent.
*/
package agent

import (
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

var (
	apiOpen = openAPIForAgent

	checkProvisionedStrategy = utils.AttemptStrategy{
		Total: 1 * time.Minute,
		Delay: 5 * time.Second,
	}
)

// openAPIForAgent exists to handle the edge case that exists
// when an environment is jumping several versions and doesn't
// yet have the environment UUID cached in the agent config.
// This happens only the first time an agent tries to connect
// after an upgrade.  If there is no environment UUID set, then
// use login version 1.
func openAPIForAgent(info *api.Info, opts api.DialOpts) (*api.State, error) {
	if info.EnvironTag.Id() == "" {
		return api.OpenWithVersion(info, opts, 1)
	}
	return api.Open(info, opts)
}

// AgentConf handles command-line flags shared by all agents.
type AgentConf struct {
	DataDir string
	mu      sync.Mutex
	_config agent.ConfigSetterWriter
}

type AgentConfigMutator func(agent.ConfigSetter) error

// AddFlags injects common agent flags into f.
func (c *AgentConf) AddFlags(f *gnuflag.FlagSet) {
	// TODO(dimitern) 2014-02-19 bug 1282025
	// We need to pass a config location here instead and
	// use it to locate the conf and the infer the data-dir
	// from there instead of passing it like that.
	f.StringVar(&c.DataDir, "data-dir", util.DataDir, "directory for juju data")
}

func (c *AgentConf) CheckArgs(args []string) error {
	if c.DataDir == "" {
		return util.RequiredError("data-dir")
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
	conf, err := agent.ReadConfig(agent.ConfigPath(c.DataDir, t))
	if err != nil {
		return err
	}
	c._config = conf
	return nil
}

func (ch *AgentConf) ChangeConfig(change AgentConfigMutator) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if err := change(ch._config); err != nil {
		return errors.Trace(err)
	}
	if err := ch._config.Write(); err != nil {
		return errors.Annotate(err, "cannot write agent configuration")
	}
	return nil
}

func (ch *AgentConf) CurrentConfig() agent.Config {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch._config.Clone()
}

// SetAPIHostPorts satisfies worker/apiaddressupdater/APIAddressSetter.
func (a *AgentConf) SetAPIHostPorts(servers [][]network.HostPort) error {
	return a.ChangeConfig(func(c agent.ConfigSetter) error {
		c.SetAPIHostPorts(servers)
		return nil
	})
}

// SetStateServingInfo satisfies worker/certupdater/SetStateServingInfo.
func (a *AgentConf) SetStateServingInfo(info params.StateServingInfo) error {
	return a.ChangeConfig(func(c agent.ConfigSetter) error {
		c.SetStateServingInfo(info)
		return nil
	})
}

type Agent interface {
	Tag() names.Tag
	ChangeConfig(AgentConfigMutator) error
}

// The AgentState interface is implemented by state types
// that represent running agents.
type AgentState interface {
	// SetAgentVersion sets the tools version that the agent is
	// currently running.
	SetAgentVersion(v version.Binary) error
	Tag() string
	Life() state.Life
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

type configChanger func(c *agent.Config)

// OpenAPIState opens the API using the given information. The agent's
// password is changed if the fallback password was used to connect to
// the API.
func OpenAPIState(agentConfig agent.Config, a Agent) (_ *api.State, _ *apiagent.Entity, outErr error) {
	info := agentConfig.APIInfo()
	st, usedOldPassword, err := openAPIStateUsingInfo(info, a, agentConfig.OldPassword())
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if outErr != nil && st != nil {
			st.Close()
		}
	}()

	entity, err := st.Agent().Entity(a.Tag())
	if err == nil && entity.Life() == params.Dead {
		logger.Errorf("agent terminating - entity %q is dead", a.Tag())
		return nil, nil, worker.ErrTerminateAgent
	}
	if params.IsCodeUnauthorized(err) {
		logger.Errorf("agent terminating due to error returned during entity lookup: %v", err)
		return nil, nil, worker.ErrTerminateAgent
	}
	if err != nil {
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
		if err := a.ChangeConfig(func(c agent.ConfigSetter) error {
			c.SetPassword(newPassword)
			c.SetOldPassword(info.Password)
			return nil
		}); err != nil {
			return nil, nil, err
		}
		if err := entity.SetPassword(newPassword); err != nil {
			return nil, nil, err
		}

		// Reconnect to the API with the new password.
		st.Close()
		info.Password = newPassword
		st, err = apiOpen(info, api.DialOpts{})
		if err != nil {
			return nil, nil, err
		}
	}

	return st, entity, err
}

// OpenAPIStateUsingInfo opens the API using the given API
// information, and returns the opened state and the api entity with
// the given tag.
func OpenAPIStateUsingInfo(info *api.Info, a Agent, oldPassword string) (*api.State, error) {
	st, _, err := openAPIStateUsingInfo(info, a, oldPassword)
	return st, err
}

func openAPIStateUsingInfo(info *api.Info, a Agent, oldPassword string) (*api.State, bool, error) {
	// We let the API dial fail immediately because the
	// runner's loop outside the caller of openAPIState will
	// keep on retrying. If we block for ages here,
	// then the worker that's calling this cannot
	// be interrupted.
	st, err := apiOpen(info, api.DialOpts{})
	usedOldPassword := false
	if params.IsCodeUnauthorized(err) {
		// We've perhaps used the wrong password, so
		// try again with the fallback password.
		infoCopy := *info
		info = &infoCopy
		info.Password = oldPassword
		usedOldPassword = true
		st, err = apiOpen(info, api.DialOpts{})
	}
	// The provisioner may take some time to record the agent's
	// machine instance ID, so wait until it does so.
	if params.IsCodeNotProvisioned(err) {
		for a := checkProvisionedStrategy.Start(); a.Next(); {
			st, err = apiOpen(info, api.DialOpts{})
			if !params.IsCodeNotProvisioned(err) {
				break
			}
		}
	}
	if err != nil {
		if params.IsCodeNotProvisioned(err) || params.IsCodeUnauthorized(err) {
			logger.Errorf("agent terminating due to error returned during API open: %v", err)
			return nil, false, worker.ErrTerminateAgent
		}
		return nil, false, err
	}

	return st, usedOldPassword, nil
}
