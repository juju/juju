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
		if c.ControllerMachines != "" {
			showHA = true
			break
		}
	}
	if showHA {
		w.Println("CONTROLLER", "MODEL", "USER", "ACCESS", "CLOUD/REGION", "MODELS", "MACHINES", "HA", "VERSION")
		tw.SetColumnAlignRight(5)
		tw.SetColumnAlignRight(6)
		tw.SetColumnAlignRight(7)
	} else {
		w.Println("CONTROLLER", "MODEL", "USER", "ACCESS", "CLOUD/REGION", "MODELS", "MACHINES", "VERSION")
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
			if promptRefresh {
				access += refresh
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
		if promptRefresh {
			agentVersion += refresh
		}
		machineCount := noValueDisplay
		if c.MachineCount != nil {
			machineCount = fmt.Sprintf("%d", *c.MachineCount)
			if promptRefresh {
				machineCount += refresh
			}
		}
		modelCount := noValueDisplay
		if c.ModelCount != nil {
			modelCount = fmt.Sprintf("%d", *c.ModelCount)
			if promptRefresh {
				modelCount += refresh
			}
		}
		w.Print(modelName, userName, access, cloudRegion, modelCount, machineCount)
		if showHA {
			controllerMachineInfo := c.ControllerMachines
			if controllerMachineInfo == "" {
				controllerMachineInfo = "1"
			}
			if promptRefresh {
				controllerMachineInfo += refresh
			}
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
	if promptRefresh && len(names) > 0 {
		fmt.Fprintln(writer, "\n+ these are the last known values, run with --refresh to see the latest information.")
	}
	return nil
}
