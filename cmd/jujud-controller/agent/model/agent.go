// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
)

// WrapAgent wraps an agent.Agent (expected to be a machine agent, fwiw)
// such that its references the supplied model rather than the controller
// model; its config is immutable; and it doesn't use OldPassword.
//
// It's a strong sign that the agent package needs some work...
func WrapAgent(a agent.Agent, controllerUUID, modelUUID string) (agent.Agent, error) {
	if !names.IsValidModel(modelUUID) {
		return nil, errors.NotValidf("model uuid %q", modelUUID)
	}
	if !names.IsValidController(controllerUUID) {
		return nil, errors.NotValidf("controller uuid %q", controllerUUID)
	}
	return &modelAgent{
		Agent:          a,
		modelUUID:      modelUUID,
		controllerUUID: controllerUUID,
	}, nil
}

type modelAgent struct {
	agent.Agent
	modelUUID      string
	controllerUUID string
}

// ChangeConfig is part of the agent.Agent interface. This implementation
// always returns an error.
func (a *modelAgent) ChangeConfig(_ agent.ConfigMutator) error {
	return errors.New("model agent config is immutable")
}

// CurrentConfig is part of the agent.Agent interface. This implementation
// returns an agent.Config that reports tweaked API connection information.
func (a *modelAgent) CurrentConfig() agent.Config {
	return &modelAgentConfig{
		Config:         a.Agent.CurrentConfig(),
		modelUUID:      a.modelUUID,
		controllerUUID: a.controllerUUID,
	}
}

type modelAgentConfig struct {
	agent.Config
	modelUUID      string
	controllerUUID string
}

// Model is part of the agent.Config interface. This implementation always
// returns the configured model tag.
func (c *modelAgentConfig) Model() names.ModelTag {
	return names.NewModelTag(c.modelUUID)
}

// Controller is part of the agent.Config interface. This implementation always
// returns the configured controller tag.
func (c *modelAgentConfig) Controller() names.ControllerTag {
	return names.NewControllerTag(c.controllerUUID)
}

// APIInfo is part of the agent.Config interface. This implementation always
// replaces the target model tag with the configured model tag.
func (c *modelAgentConfig) APIInfo() (*api.Info, bool) {
	info, ok := c.Config.APIInfo()
	if !ok {
		return nil, false
	}
	info.ModelTag = names.NewModelTag(c.modelUUID)
	return info, true
}

// OldPassword is part of the agent.Config interface. This implementation
// always returns an empty string -- which, we hope, is never valid.
func (*modelAgentConfig) OldPassword() string {
	return ""
}
