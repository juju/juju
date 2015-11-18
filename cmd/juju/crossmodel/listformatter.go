// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// formatListTabular returns a tabular summary of remote services or
// errors out if parameter is not of expected type.
func formatListTabular(value interface{}) ([]byte, error) {
	endpoints, ok := value.(map[string]directoryServices)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", endpoints, value)
	}
	return formatListEndpointsTabular(endpoints)
}

// formatListEndpointsTabular returns a tabular summary of listed services' endpoints.
func formatListEndpointsTabular(all map[string]directoryServices) ([]byte, error) {
	var out bytes.Buffer
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	headers := []string{"APPLICATION", "CHARM", "CONNECTED", "STORE", "URL", "ENDPOINT", "INTERFACE", "ROLE"}

	// Ensure directories are sorted alphabetically.
	//	directories := sortMapKeys(all)
	directories := []string{}
	for name, _ := range all {
		directories = append(directories, name)
	}
	sort.Strings(directories)

	for _, directory := range directories {
		print(directory)
		print(headers...)

		// Sort application names alphabetically.
		services := all[directory]
		applicationNames := []string{}
		for name, _ := range services {
			applicationNames = append(applicationNames, name)
		}
		sort.Strings(applicationNames)

		for _, name := range applicationNames {
			application := services[name]

			// Sort endpoints alphabetically.
			endpoints := []string{}
			for endpoint, _ := range application.Endpoints {
				endpoints = append(endpoints, endpoint)
			}
			sort.Strings(endpoints)

			for i, endpointName := range endpoints {

				endpoint := application.Endpoints[endpointName]
				if i == 0 {
					// As there is some information about service and its endpoints,
					// only display service information once
					// when the first endpoint is  displayed.
					print(name, application.CharmName, fmt.Sprintf("%v", application.UsersCount), application.Store, application.Location, endpointName, endpoint.Interface, endpoint.Role)
					continue
				}
				// Subsequent lines only need to display endpoint information.
				// This will display less noise.
				print("", "", "", "", "", endpointName, endpoint.Interface, endpoint.Role)
			}
		}
	}
	tw.Flush()

	return out.Bytes(), nil
}
