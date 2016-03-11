// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/api"
	"github.com/juju/juju/resource/cmd"
)

// DeployResources uploads the bytes for the given files to the server and
// creates pending resource metadata for the all resource mentioned in the
// metadata. It returns a map of resource name to pending resource IDs.
func DeployResources(serviceID string, cURL *charm.URL, files map[string]string, resources map[string]charmresource.Meta, conn api.Connection) (ids map[string]string, err error) {
	client, err := newAPIClient(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ids, err = cmd.DeployResources(cmd.DeployResourcesArgs{
		ServiceID:     serviceID,
		CharmURL:      cURL,
		Specified:     files,
		ResourcesMeta: resources,
		Client:        client,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ids, nil
}
