// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// formatTabular returns a tabular summary of remote services or
// errors out if parameter is not of expected type.
func formatTabular(value interface{}) ([]byte, error) {
	endpoints, ok := value.([]RemoteService)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", endpoints, value)
	}
	return formatOfferedEndpointsTabular(endpoints)
}

// formatOfferedEndpointsTabular returns a tabular summary of offered services' endpoints.
func formatOfferedEndpointsTabular(all []RemoteService) ([]byte, error) {
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

	print("SERVICE", "DESCRIPTION", "RELATION", "INTERFACE", "ROLE")

	for _, one := range all {
		serviceName := one.Service
		serviceDesc := one.Desc
		for _, endpoint := range one.Endpoints {
			print(serviceName, serviceDesc, endpoint.Name, endpoint.Interface, endpoint.Role)
			// Only print once.
			serviceName = ""
			serviceDesc = ""
		}
	}
	tw.Flush()

	return out.Bytes(), nil
}
