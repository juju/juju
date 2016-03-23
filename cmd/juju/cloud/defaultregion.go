// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

type setDefaultRegionCommand struct {
	cmd.CommandBase

	store  jujuclient.CredentialStore
	cloud  string
	region string
}

var setDefaultRegionDoc = `
The set-default-region command sets the default region for the specified cloud.

Example:
   juju set-default-region aws us-west-1
`

// NewSetDefaultRegionCommand returns a command to set the default region for a cloud.
func NewSetDefaultRegionCommand() cmd.Command {
	return &setDefaultRegionCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *setDefaultRegionCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-default-region",
		Purpose: "sets the default region for a cloud",
		Doc:     setDefaultRegionDoc,
		Args:    "<cloud> <region>",
	}
}

func (c *setDefaultRegionCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("Usage: juju set-default-region <cloud-name> <region>")
	}
	c.cloud = args[0]
	c.region = args[1]
	return cmd.CheckEmpty(args[2:])
}

func hasRegion(region string, regions []jujucloud.Region) bool {
	for _, r := range regions {
		if r.Name == region {
			return true
		}
	}
	return false
}

func (c *setDefaultRegionCommand) Run(ctxt *cmd.Context) error {
	cloudDetails, err := cloudOrProvider(c.cloud, jujucloud.CloudByName)
	if err != nil {
		return err
	}
	if !hasRegion(c.region, cloudDetails.Regions) {
		return errors.NotValidf("region %q for cloud %s", c.region, c.cloud)
	}
	var cred *jujucloud.CloudCredential
	cred, err = c.store.CredentialForCloud(c.cloud)
	if errors.IsNotFound(err) {
		cred = &jujucloud.CloudCredential{}
	} else if err != nil {
		return err
	}
	cred.DefaultRegion = c.region
	if err := c.store.UpdateCredential(c.cloud, *cred); err != nil {
		return err
	}
	ctxt.Infof("Default region in %s set to %q.", c.cloud, c.region)
	return nil
}
