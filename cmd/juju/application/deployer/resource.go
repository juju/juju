// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"strconv"

	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/v3/api/base"
	"github.com/juju/juju/v3/api/client/resources"
	resourcecmd "github.com/juju/juju/v3/cmd/juju/resource"
	"github.com/juju/juju/v3/cmd/modelcmd"
)

// DeployResourcesFunc is the function type of DeployResources.
type DeployResourcesFunc func(
	applicationID string,
	chID resources.CharmID,
	csMac *macaroon.Macaroon,
	filesAndRevisions map[string]string,
	resources map[string]charmresource.Meta,
	conn base.APICallCloser,
	filesystem modelcmd.Filesystem,
) (ids map[string]string, err error)

// DeployResources uploads the bytes for the given files to the server and
// creates pending resource metadata for the all resource mentioned in the
// metadata. It returns a map of resource name to pending resource IDs.
func DeployResources(
	applicationID string,
	chID resources.CharmID,
	csMac *macaroon.Macaroon,
	filesAndRevisions map[string]string,
	res map[string]charmresource.Meta,
	conn base.APICallCloser,
	filesystem modelcmd.Filesystem,
) (ids map[string]string, err error) {

	if len(filesAndRevisions)+len(res) == 0 {
		// Nothing to upload.
		return nil, nil
	}

	apiClient, err := resources.NewClient(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	filenames := make(map[string]string)
	revisions := make(map[string]int)
	for name, val := range filesAndRevisions {
		rev, err := strconv.Atoi(val)
		if err != nil {
			filenames[name] = val
		} else {
			revisions[name] = rev
		}
	}

	ids, err = resourcecmd.DeployResources(resourcecmd.DeployResourcesArgs{
		ApplicationID:      applicationID,
		CharmID:            chID,
		CharmStoreMacaroon: csMac,
		ResourceValues:     filenames,
		Revisions:          revisions,
		ResourcesMeta:      res,
		Client:             &deployClient{apiClient},
		Filesystem:         filesystem,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ids, nil
}

type deployClient struct {
	*resources.Client
}

// AddPendingResources adds pending metadata for store-based resources.
func (cl *deployClient) AddPendingResources(applicationID string, chID resources.CharmID, csMac *macaroon.Macaroon, res []charmresource.Resource) ([]string, error) {
	return cl.Client.AddPendingResources(resources.AddPendingResourcesArgs{
		ApplicationID:      applicationID,
		CharmID:            chID,
		CharmStoreMacaroon: csMac,
		Resources:          res,
	})
}
