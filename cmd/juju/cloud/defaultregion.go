// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/jujuclient"
)

type setDefaultRegionCommand struct {
	cmd.CommandBase

	store  jujuclient.CredentialStore
	cloud  string
	region string
}

var usageSetDefaultRegionSummary = `
Sets the default region for a cloud.`[1:]

var usageSetDefaultRegionDetails = `
The default region is specified directly as an argument.

To unset previously set default region for a cloud, use the command
without a region argument.

Examples:
    juju set-default-region azure-china chinaeast
    juju set-default-region azure-china

See also:
    add-credential`[1:]

// NewSetDefaultRegionCommand returns a command to set the default region for a cloud.
func NewSetDefaultRegionCommand() cmd.Command {
	return &setDefaultRegionCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *setDefaultRegionCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "set-default-region",
		Args:    "<cloud name> [<region>]",
		Purpose: usageSetDefaultRegionSummary,
		Doc:     usageSetDefaultRegionDetails,
	})
}

func (c *setDefaultRegionCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("Usage: juju set-default-region <cloud-name> [<region>]")
	}
	c.cloud = args[0]
	end := 1
	if len(args) > 1 {
		c.region = args[1]
		end = 2
	}
	return cmd.CheckEmpty(args[end:])
}

func (c *setDefaultRegionCommand) Run(ctxt *cmd.Context) error {
	cloudDetails, err := common.CloudOrProvider(c.cloud, jujucloud.CloudByName)
	if err != nil {
		return err
	}
	if len(cloudDetails.Regions) == 0 {
		return errors.Errorf("cloud %s has no regions", c.cloud)
	}
	msg := fmt.Sprintf("Default region for cloud %q is no longer set on this client.", c.cloud)
	if c.region != "" {
		// Ensure region exists.
		region, err := jujucloud.RegionByName(cloudDetails.Regions, c.region)
		if err != nil {
			return err
		}
		// This is needed since user may have specified UPPER cases but regions are case sensitive.
		c.region = region.Name
		msg = fmt.Sprintf("Default region in %s set to %q.", c.cloud, c.region)
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
	ctxt.Infof(msg)
	return nil
}
