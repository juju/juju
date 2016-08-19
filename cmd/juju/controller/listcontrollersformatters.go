// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/output"
)

const noValueDisplay = "-"

func formatControllersListTabular(writer io.Writer, value interface{}) error {
	controllers, ok := value.(ControllerSet)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", controllers, value)
	}
	return formatControllersTabular(writer, controllers, false)
}

func formatShowControllersTabular(writer io.Writer, value interface{}) error {
	controllers, ok := value.(map[string]ShowControllerDetails)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", controllers, value)
	}
	controllerSet := ControllerSet{
		Controllers: make(map[string]ControllerItem, len(controllers)),
	}
	for name, details := range controllers {
		serverName := ""
		// The most recently connected-to address
		// is the first in the list.
		if len(details.Details.APIEndpoints) > 0 {
			serverName = details.Details.APIEndpoints[0]
		}
		controllerSet.Controllers[name] = ControllerItem{
			ControllerUUID: details.Details.ControllerUUID,
			Server:         serverName,
			ModelName:      details.CurrentModel,
			Cloud:          details.Details.Cloud,
			CloudRegion:    details.Details.CloudRegion,
			APIEndpoints:   details.Details.APIEndpoints,
			CACert:         details.Details.CACert,
			User:           details.Account.User,
			Access:         details.Account.Access,
		}
	}
	return formatControllersTabular(writer, controllerSet, true)
}

// formatControllersTabular returns a tabular summary of controller/model items
// sorted by controller name alphabetically.
func formatControllersTabular(writer io.Writer, set ControllerSet, withAccess bool) error {
	tw := output.TabWriter(writer)
	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	if withAccess {
		print("CONTROLLER", "MODEL", "USER", "ACCESS", "CLOUD/REGION")
	} else {
		print("CONTROLLER", "MODEL", "USER", "CLOUD/REGION")
	}

	names := []string{}
	for name, _ := range set.Controllers {
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
			if c.Access != "" {
				access = c.Access
			}
		}
		if name == set.CurrentController {
			name += "*"
			output.CurrentHighlight.Fprintf(tw, "%s\t", name)
		} else {
			fmt.Fprintf(tw, "%s\t", name)
		}
		cloudRegion := c.Cloud
		if c.CloudRegion != "" {
			cloudRegion += "/" + c.CloudRegion
		}
		if withAccess {
			print(modelName, userName, access, cloudRegion)
		} else {
			print(modelName, userName, cloudRegion)
		}
	}
	tw.Flush()

	return nil
}
