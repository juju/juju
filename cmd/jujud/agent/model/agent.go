// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
)

func WrapAgent(a agent.Agent, uuid string) (agent.Agent, error) {
	if !names.IsValidModel(uuid) {
		return nil, errors.NotValidf("model uuid %q", uuid)
	}
	return &modelAgent{
		Agent: a,
		uuid:  uuid,
	}, nil
}

type modelAgent struct {
	agent.Agent
	uuid string
}

func (a *modelAgent) ChangeConfig(xxx) error {
	return errors.New("model agent config is immutable")
}

func (a *modelAgent) CurrentConfig() agent.Config {
	return &modelAgentConfig{
		Config: a.CurrentConfig(),
		uuid:   a.uuid,
	}
}

type modelAgentConfig struct {
	agent.Config
	uuid string
}

func (c *modelAgentConfig) XXX() {

}

func (c *modelAgentConfig) APIInfo() (*api.Info, bool) {

}
