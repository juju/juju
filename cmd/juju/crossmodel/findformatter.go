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

// formatFindTabular returns a tabular summary of remote services or
// errors out if parameter is not of expected type.
func formatFindTabular(value interface{}) ([]byte, error) {
	endpoints, ok := value.(map[string]RemoteServiceResult)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", endpoints, value)
	}
	return formatFoundEndpointsTabular(endpoints)
}

// formatFoundEndpointsTabular returns a tabular summary of offered services' endpoints.
func formatFoundEndpointsTabular(all map[string]RemoteServiceResult) ([]byte, error) {
	var out bytes.Buffer
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	print("URL", "INTERFACES")

	for url, one := range all {
		serviceURL := url

		interfaces := []string{}
		for name, ep := range one.Endpoints {
			interfaces = append(interfaces, fmt.Sprintf("%s:%s", ep.Interface, name))
		}
		sort.Strings(interfaces)
		print(serviceURL, strings.Join(interfaces, ", "))
	}
	tw.Flush()

	return out.Bytes(), nil
}
