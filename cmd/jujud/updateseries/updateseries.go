// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package updateseries

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	jujuos "github.com/juju/os"
	"github.com/juju/os/series"

	"github.com/juju/juju/cmd/jujud/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
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
	manager      service.SystemdServiceManager
}

var (
	systemdDir          = "/etc/systemd/system"
	systemdMultiUserDir = systemdDir + "/multi-user.target.wants"
)

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

	if c.manager == nil {
		c.manager = service.NewSystemdServiceManager(systemd.IsRunning)
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
	var err error
	var failedAgentNames []string

	if c.machineAgent, c.unitAgents, failedAgentNames, err = c.manager.FindAgents(c.dataDir); err != nil {
		return err
	} else {
		for _, name := range failedAgentNames {
			ctx.Warningf("%s is not of type Machine nor Unit, ignoring", name)
		}
	}

	fromInitSys, err := service.VersionInitSystem(c.fromSeries)
	if err != nil {
		return err
	}

	toInitSys, err := service.VersionInitSystem(c.toSeries)
	if err != nil {
		return err
	}

	jujuVersion, err := agent.GetJujuVersion(c.machineAgent, c.dataDir)
	if err != nil {
		return err
	}

	switch {
	case toInitSys == service.InitSystemUpstart:
		return errors.NewNotSupported(nil, "downgrade to series using upstart not supported")
	case toInitSys == fromInitSys && toInitSys == service.InitSystemSystemd:
		if err := c.manager.CopyAgentBinary(c.machineAgent, c.unitAgents, c.dataDir, c.toSeries, c.fromSeries, jujuVersion); err != nil {
			return err
		} else {
			ctx.Infof("successfully copied and relinked agent binaries")
		}
		if !c.startAgents {
			break
		}

		startedMachineName, startedUnitNames, err := c.manager.StartAllAgents(
			c.machineAgent,
			c.unitAgents,
			c.dataDir,
			c.toSeries,
		)
		if err != nil {
			return err
		} else {
			for _, unitName := range startedUnitNames {
				ctx.Infof("started %s service", unitName)
			}
			ctx.Infof("started %s service", startedMachineName)
		}
		ctx.Infof("all agents successfully restarted")
	case toInitSys == service.InitSystemSystemd:
		errorHappened := false
		failedAgentNames = failedAgentNames[:0]
		startedSysdServiceNames, startedSymServiceNames, failedAgentNames, err := c.manager.WriteSystemdAgents(
			c.machineAgent,
			c.unitAgents,
			c.dataDir,
			systemdDir,
			systemdMultiUserDir,
			c.toSeries,
		)
		if err != nil {
			for _, agentName := range failedAgentNames {
				ctx.Warningf("failed to write service for %s: %s", agentName, err)
			}
			ctx.Warningf("%s", err)
			errorHappened = true
		}
		for _, sysSvcName := range startedSysdServiceNames {
			ctx.Infof("wrote %s agent, enabled and linked by systemd", sysSvcName)
		}
		for _, symSvcName := range startedSymServiceNames {
			ctx.Infof("wrote %s agent, enabled and linked by symlink", symSvcName)
		}
		err = c.manager.CopyAgentBinary(
			c.machineAgent,
			c.unitAgents,
			c.dataDir,
			c.toSeries,
			c.fromSeries,
			jujuVersion,
		)
		if err != nil {
			ctx.Warningf("%s", err)
			errorHappened = true
		} else {
			ctx.Infof("successfully copied and relinked agent binaries")
		}
		switch {
		case !errorHappened && c.startAgents:
			startedMachineName, startedUnitNames, err := c.manager.StartAllAgents(
				c.machineAgent,
				c.unitAgents,
				c.dataDir,
				c.toSeries,
			)
			if err != nil {
				return err
			} else {
				for _, unitName := range startedUnitNames {
					ctx.Infof("started %s service", unitName)
				}
				if startedMachineName != "" {
					ctx.Infof("started %s service", startedMachineName)
				}
				if err != nil {
					return nil
				}
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
