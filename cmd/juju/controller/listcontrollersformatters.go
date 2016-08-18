// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

const noValueDisplay = "-"

func formatControllersListTabular(value interface{}) ([]byte, error) {
	controllers, ok := value.(ControllerSet)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", controllers, value)
	}
	return formatControllersTabular(controllers, false)
}

func formatShowControllersTabular(value interface{}) ([]byte, error) {
	controllers, ok := value.(map[string]ShowControllerDetails)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", controllers, value)
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
	return formatControllersTabular(controllerSet, true)
}

// formatControllersTabular returns a tabular summary of controller/model items
// sorted by controller name alphabetically.
func formatControllersTabular(set ControllerSet, withAccess bool) ([]byte, error) {
	var out bytes.Buffer

	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
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
		}
		cloudRegion := c.Cloud
		if c.CloudRegion != "" {
			cloudRegion += "/" + c.CloudRegion
		}
		if withAccess {
			print(name, modelName, userName, access, cloudRegion)
		} else {
			print(name, modelName, userName, cloudRegion)
		}
	}
	tw.Flush()

	return out.Bytes(), nil
}
