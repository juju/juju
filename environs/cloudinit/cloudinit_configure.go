// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/version"
)

type UserdataConfig interface {
	// Configure is a convenience function that updates the cloudinit.Config
	// with appropriate configuration. It will run ConfigureBasic() and
	// ConfigureJuju()
	Configure() error
	// ConfigureBasic updates the provided cloudinit.Config with
	// basic configuration to initialise an OS image.
	ConfigureBasic() error
	// ConfigureJuju updates the provided cloudinit.Config with configuration
	// to initialise a Juju machine agent.
	ConfigureJuju() error
	// Render renders the cloudinit/cloudbase-init userdata needed to initialize
	// the juju agent
	Render() ([]byte, error)
}

// addAgentInfo adds agent-required information to the agent's directory
// and returns the agent directory name.
func addAgentInfo(
	cfg *MachineConfig,
	c *cloudinit.Config,
	tag names.Tag,
	toolsVersion version.Number,
) (agent.Config, error) {
	acfg, err := cfg.agentConfig(tag, toolsVersion)
	if err != nil {
		return nil, err
	}
	acfg.SetValue(agent.AgentServiceName, cfg.MachineAgentServiceName)
	cmds, err := acfg.WriteCommands(cfg.Series)
	if err != nil {
		return nil, errors.Annotate(err, "failed to write commands")
	}
	c.AddScripts(cmds...)
	return acfg, nil
}

func NewUserdataConfig(cfg *MachineConfig, c *cloudinit.Config) (UserdataConfig, error) {
	operatingSystem, err := version.GetOSFromSeries(cfg.Series)
	if err != nil {
		return nil, err
	}

	switch operatingSystem {
	case version.Ubuntu:
		return newUbuntuConfig(cfg, c)
	case version.Windows:
		return newWindowsConfig(cfg, c)
	default:
		return nil, errors.Errorf("Unsupported OS %s", cfg.Series)
	}
}
