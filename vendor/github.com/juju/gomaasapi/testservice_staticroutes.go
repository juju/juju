// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

func getStaticRoutesEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/static-routes/", version)
}

// TestStaticRoute is the MAAS API Static Route representation
type TestStaticRoute struct {
	Destination TestSubnet `json:"destination"`
	Source      TestSubnet `json:"source"`
	Metric      uint       `json:"metric"`
	GatewayIP   string     `json:"gateway_ip"`
	ResourceURI string     `json:"resource_uri"`
	ID          uint       `json:"id"`
	// These are internal bookkeeping, and not part of the public API, so
	// should not be in the JSON
	sourceCIDR      string `json:"-"`
	destinationCIDR string `json:"-"`
}

// staticRoutesHandler handles requests for '/api/<version>/static-routes/'.
func staticRoutesHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	checkError(err)
	op := values.Get("op")
	if op != "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	staticRoutesURLRE := regexp.MustCompile(`/static-routes/(.+?)/`)
	staticRoutesURLMatch := staticRoutesURLRE.FindStringSubmatch(r.URL.Path)
	staticRoutesURL := getStaticRoutesEndpoint(server.version)

	var ID uint
	var gotID bool
	if staticRoutesURLMatch != nil {
		// We pass a nil mapping, as static routes don't have names, but this gives
		// consistent integers and range checking.
		ID, err = NameOrIDToID(staticRoutesURLMatch[1], nil, 1, uint(len(server.staticRoutes)))

		if err != nil {
			http.NotFoundHandler().ServeHTTP(w, r)
			return
		}

		gotID = true
	}

	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if len(server.staticRoutes) == 0 {
			// Until a static-route is created, behave as if the endpoint
			// does not exist. This way we can simulate older MAAS
			// servers that do not support spaces.
			http.NotFoundHandler().ServeHTTP(w, r)
			return
		}

		if r.URL.Path == staticRoutesURL {
			var routes []*TestStaticRoute
			// Iterating by id rather than a dictionary iteration
			// preserves the order of the routes in the result.
			for i := uint(1); i < server.nextStaticRoute; i++ {
				route, ok := server.staticRoutes[i]
				if ok {
					server.setSubnetsOnStaticRoute(route)
					routes = append(routes, route)
				}
			}
			err = json.NewEncoder(w).Encode(routes)
		} else if gotID == false {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			err = json.NewEncoder(w).Encode(server.staticRoutes[ID])
		}
		checkError(err)
	case "POST":
		w.WriteHeader(http.StatusNotImplemented)
		// TODO(jam) 2017-02-23 we could probably wire this into creating a new
		// static route if we need the support.
		//server.NewStaticRoute(r.Body)
	case "PUT":
		w.WriteHeader(http.StatusNotImplemented)
		// TODO(jam): 2017-02-23 if we wanted to implement this, something like:
		//server.UpdateStaticRoute(r.Body)
	case "DELETE":
		delete(server.staticRoutes, ID)
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// CreateStaticRoute is used to create new Static Routes on the server.
type CreateStaticRoute struct {
	SourceCIDR      string `json:"source"`
	DestinationCIDR string `json:"destination"`
	GatewayIP       string `json:"gateway_ip"`
	Metric          uint   `json:"metric"`
}

func decodePostedStaticRoute(staticRouteJSON io.Reader) CreateStaticRoute {
	var postedStaticRoute CreateStaticRoute
	decoder := json.NewDecoder(staticRouteJSON)
	err := decoder.Decode(&postedStaticRoute)
	checkError(err)
	return postedStaticRoute
}

// NewStaticRoute creates a Static Route in the test server.
func (server *TestServer) NewStaticRoute(staticRouteJSON io.Reader) *TestStaticRoute {
	postedStaticRoute := decodePostedStaticRoute(staticRouteJSON)
	// TODO(jam): 2017-02-03 Validate that sourceSubnet and destinationSubnet really do exist
	// sourceSubnet := blah
	// destinationSubnet := blah
	newStaticRoute := &TestStaticRoute{
		destinationCIDR: postedStaticRoute.DestinationCIDR,
		sourceCIDR:      postedStaticRoute.SourceCIDR,
		Metric:          postedStaticRoute.Metric,
		GatewayIP:       postedStaticRoute.GatewayIP,
	}
	newStaticRoute.ID = server.nextStaticRoute
	newStaticRoute.ResourceURI = fmt.Sprintf("/api/%s/static-routes/%d/", server.version, int(server.nextStaticRoute))
	server.staticRoutes[server.nextStaticRoute] = newStaticRoute

	server.nextStaticRoute++
	return newStaticRoute
}

// setSubnetsOnStaticRoutes fetches the subnets for the specified static route
// and adds them to it.
func (server *TestServer) setSubnetsOnStaticRoute(staticRoute *TestStaticRoute) {
	for i := uint(1); i < server.nextSubnet; i++ {
		subnet, ok := server.subnets[i]
		if ok {
			if subnet.CIDR == staticRoute.sourceCIDR {
				staticRoute.Source = subnet
			} else if subnet.CIDR == staticRoute.destinationCIDR {
				staticRoute.Destination = subnet
			}
		}
	}
}
