// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/cmd/output"
)

// formatFindTabular returns a tabular summary of remote applications or
// errors out if parameter is not of expected type.
func formatFindTabular(writer io.Writer, value interface{}) error {
	endpoints, ok := value.(map[string]ApplicationOfferResult)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", endpoints, value)
	}
	return formatFoundEndpointsTabular(writer, endpoints)
}

// formatFoundEndpointsTabular returns a tabular summary of offered applications' endpoints.
func formatFoundEndpointsTabular(writer io.Writer, all map[string]ApplicationOfferResult) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.Println("URL", "Access", "Interfaces")

	for url, one := range all {
		applicationURL := url

		interfaces := []string{}
		for name, ep := range one.Endpoints {
			interfaces = append(interfaces, fmt.Sprintf("%s:%s", ep.Interface, name))
		}
		sort.Strings(interfaces)
		w.Println(applicationURL, one.Access, strings.Join(interfaces, ", "))
	}
	tw.Flush()

	return nil
}
