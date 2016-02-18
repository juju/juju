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
	return formatControllersTabular(controllers)
}

// formatControllersTabular returns a tabular summary of controller/model items
// sorted by controller name alphabetically.
func formatControllersTabular(set ControllerSet) ([]byte, error) {
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

	print("CONTROLLER", "MODEL", "USER", "SERVER")

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
		if c.User != "" {
			userName = c.User
		}
		if name == set.CurrentController {
			name += "*"
		}
		print(name, modelName, userName, c.Server)
	}
	tw.Flush()

	return out.Bytes(), nil
}
