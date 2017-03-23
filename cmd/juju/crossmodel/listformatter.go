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

// formatListTabular returns a tabular summary of remote application offers or
// errors out if parameter is not of expected type.
func formatListTabular(writer io.Writer, value interface{}) error {
	offers, ok := value.(offeredApplications)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", offers, value)
	}
	return formatListEndpointsTabular(writer, offers)
}

type offerItems []ListOfferItem

// formatListEndpointsTabular returns a tabular summary of listed applications' endpoints.
func formatListEndpointsTabular(writer io.Writer, offers offeredApplications) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	// Sort offers by source then application name.
	allOffers := offerItems{}
	for _, offer := range offers {
		allOffers = append(allOffers, offer)
	}
	sort.Sort(allOffers)

	w.Println("Application", "Charm", "Connected", "Store", "URL", "Endpoint", "Interface", "Role")
	for _, offer := range allOffers {
		// Sort endpoints alphabetically.
		endpoints := []string{}
		for endpoint, _ := range offer.Endpoints {
			endpoints = append(endpoints, endpoint)
		}
		sort.Strings(endpoints)

		for i, endpointName := range endpoints {

			endpoint := offer.Endpoints[endpointName]
			if i == 0 {
				// As there is some information about offer and its endpoints,
				// only display offer information once when the first endpoint is displayed.
				w.Println(offer.ApplicationName, offer.CharmName, fmt.Sprint(offer.UsersCount), offer.Source, offer.Location, endpointName, endpoint.Interface, endpoint.Role)
				continue
			}
			// Subsequent lines only need to display endpoint information.
			// This will display less noise.
			w.Println("", "", "", "", "", endpointName, endpoint.Interface, endpoint.Role)
		}
	}
	tw.Flush()
	return nil
}

func (o offerItems) Len() int      { return len(o) }
func (o offerItems) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o offerItems) Less(i, j int) bool {
	if o[i].Source == o[j].Source {
		return o[i].ApplicationName < o[j].ApplicationName
	}
	return o[i].Source < o[j].Source
}
