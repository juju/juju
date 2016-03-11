// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/resource/api/client"
	"github.com/juju/juju/resource/cmd"
)

// DeployResources uploads the bytes for the given files to the server and
// creates pending resource metadata for the all resource mentioned in the
// metadata. It returns a map of resource name to pending resource IDs.
func DeployResources(serviceID string, cURL *charm.URL, csMac *macaroon.Macaroon, files map[string]string, resources map[string]charmresource.Meta, conn api.Connection) (ids map[string]string, err error) {
	client, err := newAPIClient(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ids, err = cmd.DeployResources(cmd.DeployResourcesArgs{
		ServiceID:          serviceID,
		CharmURL:           cURL,
		CharmStoreMacaroon: csMac,
		Specified:          files,
		ResourcesMeta:      resources,
		Client:             &deployClient{client},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ids, nil
}

type deployClient struct {
	*client.Client
}

// AddPendingResources adds pending metadata for store-based resources.
func (cl *deployClient) AddPendingResources(serviceID string, cURL *charm.URL, csMac *macaroon.Macaroon, resources []charmresource.Resource) ([]string, error) {
	return cl.Client.AddPendingResources(client.AddPendingResourcesArgs{
		ServiceID:          serviceID,
		CharmURL:           cURL,
		CharmStoreMacaroon: csMac,
		Resources:          resources,
	})
}
