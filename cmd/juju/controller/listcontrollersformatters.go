// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"io"
	"sort"

	"github.com/juju/errors"

	"github.com/juju/juju/core/output"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/version"
)

const (
	noValueDisplay  = "-"
	notKnownDisplay = "(unknown)"
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

	if promptRefresh && len(set.Controllers) > 0 {
		fmt.Fprintln(writer, "Use --refresh option with this command to see the latest information.")
		fmt.Fprintln(writer)
	}
	w.Println("Controller", "Model", "User", "Access", "Cloud/Region", "Models", "Nodes", "HA", "Version")
	tw.SetColumnAlignRight(5)
	tw.SetColumnAlignRight(6)
	tw.SetColumnAlignRight(7)

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
		if c.NodeCount != nil && *c.NodeCount > 0 {
			machineCount = fmt.Sprintf("%d", *c.NodeCount)
		}
		modelCount := noValueDisplay
		if c.ModelCount != nil && *c.ModelCount > 0 {
			modelCount = fmt.Sprintf("%d", *c.ModelCount)
		}
		w.Print(modelName, userName, access, cloudRegion, modelCount, machineCount)
		controllerMachineInfo, warn := controllerMachineStatus(c.ControllerMachines)
		if warn {
			w.PrintColor(output.WarningHighlight, controllerMachineInfo)
		} else {
			w.Print(controllerMachineInfo)
		}
		if staleVersion {
			w.PrintColor(output.WarningHighlight, agentVersion)
		} else {
			w.Print(agentVersion)
		}
		w.Println()
	}
	tw.Flush()
	return nil
}

func controllerMachineStatus(machines *ControllerMachines) (string, bool) {
	if machines == nil || machines.Total == 0 {
		return "-", false
	}
	if machines.Total == 1 {
		return "none", false
	}
	controllerMachineStatus := ""
	warn := machines.Active < machines.Total
	controllerMachineStatus = fmt.Sprintf("%d", machines.Total)
	if machines.Active < machines.Total {
		controllerMachineStatus = fmt.Sprintf("%d/%d", machines.Active, machines.Total)
	}
	return controllerMachineStatus, warn
}
