// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

func formatControllersListTabular(value interface{}) ([]byte, error) {
	controllers, ok := value.(controllerList)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", controllers, value)
	}
	return formatControllersTabular(controllers)
}

// formatControllersTabular returns a tabular summary of contorller/model items.
func formatControllersTabular(controllers controllerList) ([]byte, error) {
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

	for _, c := range controllers {
		print(c.ControllerName, c.ModelName, c.User, c.Server)
	}
	tw.Flush()

	return out.Bytes(), nil
}
