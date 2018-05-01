// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package updateseries

import (
	"os"
	"path"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/fs"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/symlink"
	"github.com/juju/version"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/cmd/jujud/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	agentinfo "github.com/juju/juju/core/agent"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/systemd"
)

var logger = loggo.GetLogger("juju.cmd.jujud.updateseries")

const updateSeriesCommandDoc = `
Update Juju agents on this machine to start after series upgrade.
`

type UpdateSeriesCommand struct {
	cmd.CommandBase

	machineAgent string
	unitAgents   []string
	dataDir      string
	toSeries     string
	fromSeries   string
	startAgents  bool
}

func (c *UpdateSeriesCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-updateseries",
		Args:    "--to-series <series> --from-series <series> [--data-dir <dir>|--start-agents]",
		Purpose: "Update Juju agents on this machine to start after series upgrade",
		Doc:     updateSeriesCommandDoc,
	}
}

func (c *UpdateSeriesCommand) Init(args []string) error {
	switch {
	case c.toSeries == "" && c.fromSeries == "":
		return errors.Errorf("both --to-series and --from-series must be specified")
	case c.toSeries == "":
		return errors.Errorf("--to-series must be specified")
	case c.fromSeries == "":
		return errors.Errorf("--from-series must be specified")
	case c.toSeries == c.fromSeries:
		return errors.Errorf("--to-series and --from-series cannot be the same")
	}

	fromOS, err1 := series.GetOSFromSeries(c.toSeries)
	toOS, err2 := series.GetOSFromSeries(c.fromSeries)
	switch {
	case err1 != nil:
		return err1
	case err2 != nil:
		return err2
	case fromOS != toOS:
		return errors.Errorf("series from two different operating systems specified")
	case fromOS == jujuos.Windows:
		return errors.NewNotSupported(nil, "windows not supported")
	}

	ctlr, err := isController()
	switch {
	case err != nil:
		return err
	case ctlr:
		return errors.Errorf("cannot run on a controller machine")
	}

	return c.CommandBase.Init(args)
}

func (c *UpdateSeriesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.dataDir, "data-dir", cmdutil.DataDir, "Juju base data directory")
	f.StringVar(&c.toSeries, "to-series", "", "Series updating to")
	f.StringVar(&c.fromSeries, "from-series", "", "Series updating from")
	f.BoolVar(&c.startAgents, "start-agents", false, "start agents if successful with update")
}

func (c *UpdateSeriesCommand) Run(ctx *cmd.Context) error {
	if err := agentinfo.FindAgents(&c.machineAgent, &c.unitAgents, c.dataDir); err != nil {
		return err
	}

	fromInitSys, err := service.VersionInitSystem(c.fromSeries)
	if err != nil {
		return err
	}

	toInitSys, err := service.VersionInitSystem(c.toSeries)
	if err != nil {
		return err
	}

	switch {
	case toInitSys == service.InitSystemUpstart:
		return errors.NewNotSupported(nil, "downgrade to series using upstart not supported")
	case toInitSys == fromInitSys && toInitSys == service.InitSystemSystemd:
		if err = c.copyTools(); err != nil {
			return err
		} else {
			ctx.Infof("successfully copied tools and relinked agent tools")
		}
		if !c.startAgents {
			break
		}
		if err = c.startAllAgents(ctx); err != nil {
			return err
		}
		ctx.Infof("all agents successfully restarted")
	case toInitSys == service.InitSystemSystemd:
		errorHappened := false
		if err = c.writeSystemdAgents(ctx); err != nil {
			ctx.Warningf("%s", err)
			errorHappened = true
		}
		if err = c.copyTools(); err != nil {
			ctx.Warningf("%s", err)
			errorHappened = true
		} else {
			ctx.Infof("successfully copied tools and relinked agent tools")
		}
		switch {
		case !errorHappened && c.startAgents:
			if err = c.startAllAgents(ctx); err != nil {
				return err
			}
			ctx.Infof("all agents successfully restarted")
		case errorHappened && c.startAgents:
			return errors.Errorf("unable to start agents due to previous errors")
		}
	default:
		return errors.Errorf("Failed to migrate from %s to %s", fromInitSys, toInitSys)
	}
	return nil
}

var isController = func() (bool, error) {
	services, err := service.ListServices()
	if err != nil {
		return true, err
	}
	for _, service := range services {
		if strings.HasPrefix(service, mongo.ServiceName) {
			return true, nil
		}
	}
	return false, nil
}

var (
	systemdDir          = "/etc/systemd/system"
	systemdMultiUserDir = systemdDir + "/multi-user.target.wants"
)

// For testing
var sysdIsRunning = systemd.IsRunning

func (c *UpdateSeriesCommand) writeSystemdAgents(ctx *cmd.Context) error {
	var lastError error
	for _, agentName := range append(c.unitAgents, c.machineAgent) {
		conf, err := agentinfo.CreateAgentConf(agentName, c.dataDir, c.toSeries)
		if err != nil {
			ctx.Warningf("%s", err)
			lastError = err
			continue
		}
		svcName := "jujud-" + agentName
		svc, err := service.NewService(svcName, conf, c.toSeries)

		upgradableSvc, ok := svc.(service.UpgradableService)
		if !ok {
			initName, err := service.VersionInitSystem(c.toSeries)
			if err != nil {
				return errors.Trace(errors.Annotate(err, "nor is service an UpgradableService"))
			}
			return errors.Errorf("%s service not of type UpgradableService", initName)
		}
		if err = upgradableSvc.WriteService(); err != nil {
			ctx.Warningf("failed to write service for %s: %s", agentName, err)
			lastError = err
			continue
		}

		running, err := sysdIsRunning()
		switch {
		case err != nil:
			return errors.Errorf("failure attempting to determine if systemd is running: %#v\n", err)
		case running:
			// Links for manual and automatic use of the service
			// have been written, move to the next.
			ctx.Infof("wrote %s agent, enabled and linked by systemd", svcName)
			continue
		}

		svcFileName := svcName + ".service"
		if err = os.Symlink(path.Join(c.dataDir, "init", svcName, svcFileName),
			path.Join(systemdDir, svcFileName)); err != nil && !os.IsExist(err) {
			return errors.Errorf("failed to link service file (%s) in systemd dir: %s\n", svcFileName, err)
		}
		if err = os.Symlink(path.Join(c.dataDir, "init", svcName, svcFileName),
			path.Join(systemdMultiUserDir, svcFileName)); err != nil && !os.IsExist(err) {
			return errors.Errorf("failed to link service file (%s) in multi-user.target.wants dir: %s\n", svcFileName, err)
		}
		ctx.Infof("wrote %s agent, enabled and linked by symlink", svcName)
	}
	return lastError
}

func (c *UpdateSeriesCommand) startAllAgents(ctx *cmd.Context) error {
	running, err := sysdIsRunning()
	switch {
	case err != nil:
		return err
	case !running:
		return errors.Errorf("systemd is not fully running, please reboot to start agents")
	}

	for _, unit := range c.unitAgents {
		if err = c.startAgent(unit, agentinfo.AgentKindUnit); err != nil {
			return errors.Annotatef(err, "failed to start %s service", "jujud-"+unit)
		}
		ctx.Infof("started %s service", "jujud-"+unit)
	}

	err = c.startAgent(c.machineAgent, agentinfo.AgentKindMachine)
	if err == nil {
		ctx.Infof("started %s service", "jujud-"+c.machineAgent)
	}
	return errors.Annotatef(err, "failed to start %s service", "jujud-"+c.machineAgent)
}

func (c *UpdateSeriesCommand) startAgent(name string, kind agentinfo.AgentKind) (err error) {
	renderer, err := shell.NewRenderer("")
	if err != nil {
		return err
	}
	info := agentinfo.NewAgentInfo(
		kind,
		name,
		c.dataDir,
		paths.MustSucceed(paths.LogDir(c.toSeries)),
	)
	conf := agentinfo.AgentConf(info, renderer)
	svcName := "jujud-" + name
	svc, err := service.NewService(svcName, conf, c.toSeries)
	if err = svc.Start(); err != nil {
		return err
	}
	return nil
}

func (c *UpdateSeriesCommand) copyTools() (err error) {
	defer func() {
		if err != nil {
			errors.Annotate(err, "failed to copy tools")
		}
	}()

	// Get the current juju version from the machine agent
	// conf file.
	agentConf := agent.NewAgentConf(c.dataDir)
	if err = agentConf.ReadConfig(c.machineAgent); err != nil {
		return err
	}
	config := agentConf.CurrentConfig()
	if config == nil {
		return errors.Errorf("%s agent conf is not found", c.machineAgent)
	}
	jujuVersion := config.UpgradedToVersion()

	// Setup new and old version.Binarys with only the series
	// different.
	fromVers := version.Binary{
		Number: jujuVersion,
		Arch:   arch.HostArch(),
		Series: c.fromSeries,
	}
	toVers := version.Binary{
		Number: jujuVersion,
		Arch:   arch.HostArch(),
		Series: c.toSeries,
	}

	// If tools with the new series don't already exist, copy
	// current tools to new directory with correct series.
	if _, err = os.Stat(tools.SharedToolsDir(c.dataDir, toVers)); err != nil {
		// Copy tools to new directory with correct series.
		if err = fs.Copy(tools.SharedToolsDir(c.dataDir, fromVers), tools.SharedToolsDir(c.dataDir, toVers)); err != nil {
			return err
		}
	}

	// Write tools metadata with new version, however don't change
	// the URL, so we know where it came from.
	jujuTools, err := tools.ReadTools(c.dataDir, toVers)
	if err != nil {
		return errors.Trace(err)
	}

	// Only write once
	if jujuTools.Version != toVers {
		jujuTools.Version = toVers
		if err = tools.WriteToolsMetadataData(tools.ToolsDir(c.dataDir, toVers.String()), jujuTools); err != nil {
			return err
		}
	}

	// Update Agent Tool links
	var lastError error
	for _, agentName := range append(c.unitAgents, c.machineAgent) {
		toolPath := tools.ToolsDir(c.dataDir, toVers.String())
		toolsDir := tools.ToolsDir(c.dataDir, agentName)

		err = symlink.Replace(toolsDir, toolPath)
		if err != nil {
			lastError = err
		}
	}

	return lastError
}
