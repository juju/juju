// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/replicaset"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/mongo"
)

var logger = loggo.GetLogger("juju.api.highavailability")

// Client provides access to the high availability service, used to manage controllers.
type Client struct {
	base.ClientFacade
	facade   base.FacadeCaller
	modelTag names.ModelTag
}

// NewClient returns a new HighAvailability client.
func NewClient(caller base.APICallCloser) *Client {
	modelTag, err := caller.ModelTag()
	if err != nil {
		logger.Errorf("ignoring invalid model tag: %v", err)
	}
	frontend, backend := base.NewClientFacade(caller, "HighAvailability")
	return &Client{ClientFacade: frontend, facade: backend, modelTag: modelTag}
}

// EnableHA ensures the availability of Juju controllers.
func (c *Client) EnableHA(
	numControllers int, cons constraints.Value, series string, placement []string,
) (params.ControllersChanges, error) {

	var results params.ControllersChangeResults
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{{
			ModelTag:       c.modelTag.String(),
			NumControllers: numControllers,
			Constraints:    cons,
			Series:         series,
			Placement:      placement,
		}}}

	err := c.facade.FacadeCall("EnableHA", arg, &results)
	if err != nil {
		return params.ControllersChanges{}, err
	}
	if len(results.Results) != 1 {
		return params.ControllersChanges{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.ControllersChanges{}, result.Error
	}
	return result.Result, nil
}

// MongoUpgradeMode will make all Slave members of the HA
// to shut down their mongo server.
func (c *Client) MongoUpgradeMode(v mongo.Version) (params.MongoUpgradeResults, error) {
	arg := params.UpgradeMongoParams{
		Target: v,
	}
	results := params.MongoUpgradeResults{}
	if err := c.facade.FacadeCall("StopHAReplicationForUpgrade", arg, &results); err != nil {
		return results, errors.Annotate(err, "cannnot enter mongo upgrade mode")
	}
	return results, nil
}

// ResumeHAReplicationAfterUpgrade makes all members part of HA again.
func (c *Client) ResumeHAReplicationAfterUpgrade(members []replicaset.Member) error {
	arg := params.ResumeReplicationParams{
		Members: members,
	}
	if err := c.facade.FacadeCall("ResumeHAReplicationAfterUpgrad", arg, nil); err != nil {
		return errors.Annotate(err, "cannnot resume ha")
	}
	return nil
}
