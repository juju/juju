// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/testing"
)

type mockAgentConfig struct {
	agent.Config
}

func (m *mockAgentConfig) Controller() names.ControllerTag {
	return coretesting.ControllerTag
}

func (m *mockAgentConfig) DataDir() string {
	return "/var/lib/juju"
}

func (m *mockAgentConfig) LogDir() string {
	return "/var/log/juju"
}

func (m *mockAgentConfig) OldPassword() string {
	return "old password"
}

func (m *mockAgentConfig) CACert() string {
	return coretesting.CACert
}
