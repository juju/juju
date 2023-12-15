// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3/voyeur"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/containeragent/utils"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/worker/logsender"
)

type (
	ManifoldsConfig    = manifoldsConfig
	ContainerUnitAgent = containerUnitAgent
)

type ContainerUnitAgentTest interface {
	cmd.Command
	DataDir() string
	SetAgentConf(cfg agentconf.AgentConf)
	ChangeConfig(change agent.ConfigMutator) error
	CurrentConfig() agent.Config
	Tag() names.UnitTag
	CharmModifiedVersion() int
	GetContainerNames() []string
}

func NewForTest(
	ctx *cmd.Context,
	bufferedLogger *logsender.BufferedLogWriter,
	configChangedVal *voyeur.Value,
	fileReaderWriter utils.FileReaderWriter,
	environment utils.Environment,
) ContainerUnitAgentTest {
	return &containerUnitAgent{
		ctx:              ctx,
		AgentConf:        agentconf.NewAgentConf(""),
		bufferedLogger:   bufferedLogger,
		configChangedVal: configChangedVal,
		fileReaderWriter: fileReaderWriter,
		environment:      environment,
	}
}

func (c *containerUnitAgent) SetAgentConf(cfg agentconf.AgentConf) {
	c.AgentConf = cfg
}

func (c *containerUnitAgent) GetContainerNames() []string {
	return c.containerNames
}

func (c *containerUnitAgent) DataDir() string {
	return c.AgentConf.DataDir()
}

func EnsureAgentConf(ac agentconf.AgentConf) error {
	return ensureAgentConf(ac)
}
