// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

type Agent interface {
	Tag() names.Tag
	ChangeConfig(agent.ConfigMutator) error
}

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

	if !usedOldPassword {
		// Call set password with the current password.  If we've recently
		// become a state server, this will fix up our credentials in mongo.
		if err := entity.SetPassword(info.Password); err != nil {
			return nil, nil, errors.Annotate(err, "can't reset agent password")
		}
	} else {
		// We succeeded in connecting with the fallback
		// password, so we need to create a new password
		// for the future.
		newPassword, err := utils.RandomPassword()
		if err != nil {
			return nil, nil, err
		}
		err = setAgentPassword(newPassword, info.Password, a, entity)
		if err != nil {
			return nil, nil, err
		}

		// Reconnect to the API with the new password.
		st.Close()
		info.Password = newPassword
		st, err = openAPIForAgent(info, api.DialOpts{})
		if err != nil {
			return nil, nil, err
		}
	}

	return st, entity, err
}

func setAgentPassword(newPw, oldPw string, a Agent, entity *apiagent.Entity) error {
	// Change the configuration *before* setting the entity
	// password, so that we avoid the possibility that
	// we might successfully change the entity's
	// password but fail to write the configuration,
	// thus locking us out completely.
	if err := a.ChangeConfig(func(c agent.ConfigSetter) error {
		c.SetPassword(newPw)
		c.SetOldPassword(oldPw)
		return nil
	}); err != nil {
		return err
	}
	return entity.SetPassword(newPw)
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
	st, err := openAPIForAgent(info, api.DialOpts{})
	usedOldPassword := false
	if params.IsCodeUnauthorized(err) {
		// We've perhaps used the wrong password, so
		// try again with the fallback password.
		infoCopy := *info
		info = &infoCopy
		info.Password = oldPassword
		usedOldPassword = true
		st, err = openAPIForAgent(info, api.DialOpts{})
	}

	// The provisioner may take some time to record the agent's
	// machine instance ID, so wait until it does so.
	if params.IsCodeNotProvisioned(err) {
		for a := checkProvisionedStrategy.Start(); a.Next(); {
			st, err = openAPIForAgent(info, api.DialOpts{})
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

var (
	// openAPIForAgent exists to handle the edge case that exists
	// when an environment is jumping several versions and doesn't
	// yet have the environment UUID cached in the agent config.
	// This happens only the first time an agent tries to connect
	// after an upgrade.  If there is no environment UUID set, then
	// use login version 1.
	openAPIForAgent = func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		if info.EnvironTag.Id() == "" {
			return api.OpenWithVersion(info, opts, 1)
		}
		return api.Open(info, opts)
	}

	checkProvisionedStrategy = utils.AttemptStrategy{
		Total: 1 * time.Minute,
		Delay: 5 * time.Second,
	}
)
