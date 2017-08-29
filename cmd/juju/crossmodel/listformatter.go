// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"io"
	"sort"

	"github.com/juju/ansiterm"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/relation"
)

// formatListSummary returns a tabular summary of remote application offers or
// errors out if parameter is not of expected type.
func formatListSummary(writer io.Writer, value interface{}) error {
	offers, ok := value.(offeredApplications)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", offers, value)
	}
	return formatListEndpointsSummary(writer, offers)
}

type offerItems []ListOfferItem

// formatListEndpointsSummary returns a tabular summary of listed applications' endpoints.
func formatListEndpointsSummary(writer io.Writer, offers offeredApplications) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	// Sort offers by source then application name.
	allOffers := offerItems{}
	for _, offer := range offers {
		allOffers = append(allOffers, offer)
	}
	sort.Sort(allOffers)

	w.Println("Offer", "Application", "Charm", "Connected", "Store", "URL", "Endpoint", "Interface", "Role")
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
				connectedCount := len(offer.Connections)
				w.Println(offer.OfferName, offer.ApplicationName, offer.CharmURL, fmt.Sprint(connectedCount),
					offer.Source, offer.OfferURL, endpointName, endpoint.Interface, endpoint.Role)
				continue
			}
			// Subsequent lines only need to display endpoint information.
			// This will display less noise.
			w.Println("", "", "", "", "", "", endpointName, endpoint.Interface, endpoint.Role)
		}
	}
	tw.Flush()
	return nil
}

func (o offerItems) Len() int      { return len(o) }
func (o offerItems) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o offerItems) Less(i, j int) bool {
	if o[i].Source == o[j].Source {
		return o[i].OfferName < o[j].OfferName
	}
	return o[i].Source < o[j].Source
}

func formatListTabular(writer io.Writer, value interface{}) error {
	offers, ok := value.(offeredApplications)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", offers, value)
	}
	return formatListEndpointsTabular(writer, offers)
}

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

	w.Println("Offer", "User", "Relation id", "Status", "Endpoint", "Interface", "Role")
	for _, offer := range allOffers {
		// Sort endpoints alphabetically.
		endpoints := []string{}
		for endpoint, _ := range offer.Endpoints {
			endpoints = append(endpoints, endpoint)
		}
		sort.Strings(endpoints)

		// Sort connections by relation id and username.
		sort.Sort(byUserRelationId(offer.Connections))

		for i, conn := range offer.Connections {
			if i == 0 {
				w.Print(offer.OfferName)
			} else {
				w.Print("")
			}
			endpoints := make(map[string]RemoteEndpoint)
			for alias, ep := range offer.Endpoints {
				aliasedEp := ep
				aliasedEp.Name = alias
				endpoints[ep.Name] = ep
			}
			connEp := endpoints[conn.Endpoint]
			w.Print(conn.Username, conn.RelationId)
			w.PrintColor(RelationStatusColor(relation.Status(conn.Status)), conn.Status)
			w.Println(connEp.Name, connEp.Interface, connEp.Role)
		}
	}
	tw.Flush()
	return nil
}

// RelationStatusColor returns a context used to print the status with the relevant color.
func RelationStatusColor(status relation.Status) *ansiterm.Context {
	switch status {
	case relation.Joined:
		return output.GoodHighlight
	case relation.Suspended:
		return output.WarningHighlight
	case relation.Broken:
		return output.ErrorHighlight
	}
	return nil
}

type byUserRelationId []offerConnectionStatus

func (b byUserRelationId) Len() int {
	return len(b)
}

func (b byUserRelationId) Less(i, j int) bool {
	if b[i].Username == b[j].Username {
		return b[i].RelationId < b[j].RelationId
	}
	return b[i].Username < b[j].Username
}

func (b byUserRelationId) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
