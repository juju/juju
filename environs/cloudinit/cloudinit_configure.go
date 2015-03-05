// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"fmt"

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

func NewUserdataConfig(mcfg *MachineConfig, conf *cloudinit.Config) (UserdataConfig, error) {
	// TODO(ericsnow) bug #1426217
	// Protect mcfg and conf better.
	operatingSystem, err := version.GetOSFromSeries(mcfg.Series)
	if err != nil {
		return nil, err
	}

	base := baseConfigure{
		mcfg: mcfg,
		conf: conf,
		os:   operatingSystem,
	}
	if err := base.init(); err != nil {
		return nil, errors.Trace(err)
	}

	switch operatingSystem {
	case version.Ubuntu:
		return &ubuntuConfigure{base}, nil
	case version.Windows:
		return &windowsConfigure{base}, nil
	default:
		return nil, errors.NotSupportedf("OS %s", mcfg.Series)
	}
}

type baseConfigure struct {
	mcfg     *MachineConfig
	conf     *cloudinit.Config
	renderer cloudinit.Renderer
	os       version.OSType
}

func (c *baseConfigure) init() error {
	renderer, err := cloudinit.NewRenderer(c.mcfg.Series)
	if err != nil {
		return err
	}
	c.renderer = renderer
	return nil
}

func (c *baseConfigure) Render() ([]byte, error) {
	return c.renderer.Render(c.conf)
}

func (c *baseConfigure) addMachineAgentToBoot(name string) error {
	svc, toolsDir, err := c.mcfg.initService()
	if err != nil {
		return errors.Trace(err)
	}

	// Make the agent run via a symbolic link to the actual tools
	// directory, so it can upgrade itself without needing to change
	// the init script.
	c.conf.AddScripts(c.toolsSymlinkCommand(toolsDir))

	cmds, err := svc.InstallCommands()
	if err != nil {
		return errors.Annotatef(err, "cannot make cloud-init init script for the %s agent", name)
	}
	startCmds, err := svc.StartCommands()
	if err != nil {
		return errors.Annotatef(err, "cannot make cloud-init init script for the %s agent", name)
	}
	cmds = append(cmds, startCmds...)

	svcName := c.mcfg.MachineAgentServiceName
	c.conf.AddRunCmd(cloudinit.LogProgressCmd("Starting Juju machine agent (%s)", svcName))
	c.conf.AddScripts(cmds...)
	return nil
}

func (c *baseConfigure) toolsSymlinkCommand(toolsDir string) string {
	switch c.os {
	case version.Windows:
		return fmt.Sprintf(
			`cmd.exe /C mklink /D %s %v`,
			c.renderer.FromSlash(toolsDir),
			c.mcfg.Tools.Version,
		)
	default:
		// TODO(dfc) ln -nfs, so it doesn't fail if for some reason that
		// the target already exists.
		return fmt.Sprintf(
			"ln -s %v %s",
			c.mcfg.Tools.Version,
			shquote(toolsDir),
		)
	}
}
