// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"io"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/output"
)

// formatListTabular returns a tabular summary of remote applications or
// errors out if parameter is not of expected type.
func formatListTabular(writer io.Writer, value interface{}) error {
	endpoints, ok := value.(map[string]directoryApplications)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", endpoints, value)
	}
	return formatListEndpointsTabular(writer, endpoints)
}

// formatListEndpointsTabular returns a tabular summary of listed applications' endpoints.
func formatListEndpointsTabular(writer io.Writer, all map[string]directoryApplications) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	// Ensure directories are sorted alphabetically.
	directories := []string{}
	for name, _ := range all {
		directories = append(directories, name)
	}
	sort.Strings(directories)

	for _, directory := range directories {
		w.Println(directory)
		w.Println("Application", "Charm", "Connected", "Store", "URL", "Endpoint", "Interface", "Role")

		// Sort application names alphabetically.
		applications := all[directory]
		applicationNames := []string{}
		for name, _ := range applications {
			applicationNames = append(applicationNames, name)
		}
		sort.Strings(applicationNames)

		for _, name := range applicationNames {
			application := applications[name]

			// Sort endpoints alphabetically.
			endpoints := []string{}
			for endpoint, _ := range application.Endpoints {
				endpoints = append(endpoints, endpoint)
			}
			sort.Strings(endpoints)

			for i, endpointName := range endpoints {

				endpoint := application.Endpoints[endpointName]
				if i == 0 {
					// As there is some information about application and its endpoints,
					// only display application information once
					// when the first endpoint is  displayed.
					w.Println(name, application.CharmName, fmt.Sprint(application.UsersCount), application.Store, application.Location, endpointName, endpoint.Interface, endpoint.Role)
					continue
				}
				// Subsequent lines only need to display endpoint information.
				// This will display less noise.
				w.Println("", "", "", "", "", endpointName, endpoint.Interface, endpoint.Role)
			}
		}
	}
	tw.Flush()
	return nil
}
