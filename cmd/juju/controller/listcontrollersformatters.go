// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"io"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/cmd/output"
	jujuversion "github.com/juju/juju/version"
)

const (
	noValueDisplay  = "-"
	notKnownDisplay = "(unknown)"
	refresh         = "+"
)

func (c *listControllersCommand) formatControllersListTabular(writer io.Writer, value interface{}) error {
	controllers, ok := value.(ControllerSet)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", controllers, value)
	}
	return formatControllersTabular(writer, controllers, !c.refresh)
}

// formatControllersTabular returns a tabular summary of controller/model items
// sorted by controller name alphabetically.
func formatControllersTabular(writer io.Writer, set ControllerSet, promptRefresh bool) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	// See if we need the HA column.
	showHA := false
	for _, c := range set.Controllers {
		if c.ControllerMachines != nil && c.ControllerMachines.Total > 1 {
			showHA = true
			break
		}
	}

	p := func(headers ...interface{}) {
		if promptRefresh && len(set.Controllers) > 0 {
			for i, h := range headers {
				switch h {
				case "ACCESS", "MACHINES", "MODELS", "HA", "VERSION":
					h = h.(string) + refresh
				}
				headers[i] = h
			}
		}
		w.Println(headers...)
	}

	if showHA {
		p("CONTROLLER", "MODEL", "USER", "ACCESS", "CLOUD/REGION", "MODELS", "MACHINES", "HA", "VERSION")
		tw.SetColumnAlignRight(5)
		tw.SetColumnAlignRight(6)
		tw.SetColumnAlignRight(7)
	} else {
		p("CONTROLLER", "MODEL", "USER", "ACCESS", "CLOUD/REGION", "MODELS", "MACHINES", "VERSION")
		tw.SetColumnAlignRight(5)
		tw.SetColumnAlignRight(6)
	}

	names := []string{}
	for name := range set.Controllers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c := set.Controllers[name]
		modelName := noValueDisplay
		if c.ModelName != "" {
			modelName = c.ModelName
		}
		userName := noValueDisplay
		access := noValueDisplay
		if c.User != "" {
			userName = c.User
			access = notKnownDisplay
			if c.Access != "" {
				access = c.Access
			}
		}
		if name == set.CurrentController {
			name += "*"
			w.PrintColor(output.CurrentHighlight, name)
		} else {
			w.Print(name)
		}
		cloudRegion := c.Cloud
		if c.CloudRegion != "" {
			cloudRegion += "/" + c.CloudRegion
		}
		agentVersion := c.AgentVersion
		staleVersion := false
		if agentVersion == "" {
			agentVersion = notKnownDisplay
		} else {
			agentVersionNum, err := version.Parse(agentVersion)
			staleVersion = err == nil && jujuversion.Current.Compare(agentVersionNum) > 0
		}
		machineCount := noValueDisplay
		if c.MachineCount != nil && *c.MachineCount > 0 {
			machineCount = fmt.Sprintf("%d", *c.MachineCount)
		}
		modelCount := noValueDisplay
		if c.ModelCount != nil && *c.ModelCount > 0 {
			modelCount = fmt.Sprintf("%d", *c.ModelCount)
		}
		w.Print(modelName, userName, access, cloudRegion, modelCount, machineCount)
		if showHA {
			controllerMachineInfo, warn := controllerMachineStatus(c.ControllerMachines)
			if warn {
				w.PrintColor(output.WarningHighlight, controllerMachineInfo)
			} else {
				w.Print(controllerMachineInfo)
			}
		}
		if staleVersion {
			w.PrintColor(output.WarningHighlight, agentVersion)
		} else {
			w.Print(agentVersion)
		}
		w.Println()
	}
	tw.Flush()
	if promptRefresh && len(names) > 0 {
		fmt.Fprintln(writer, "\n+ these are the last known values, run with --refresh to see the latest information.")
	}
	return nil
}

func controllerMachineStatus(machines *ControllerMachines) (string, bool) {
	if machines == nil || machines.Total == 0 {
		return "-", false
	}
	controllerMachineStatus := ""
	warn := machines.Active < machines.Total
	controllerMachineStatus = fmt.Sprintf("%d", machines.Total)
	if machines.Active < machines.Total {
		controllerMachineStatus = fmt.Sprintf("%d/%d", machines.Active, machines.Total)
	}
	return controllerMachineStatus, warn
}
