// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

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
	reset  bool
}

var usageSetDefaultRegionSummary = `
Gets, sets, or unsets the default region for a cloud on this client.`[1:]

var usageSetDefaultRegionDetails = `
`[1:]

const usageSetDefaultRegionnExamples = `
Set the default region for the ` + "`azure-china`" + ` cloud to ` + "`chinaeast`" + `:

    juju default-region azure-china chinaeast

Get the default region for the ` + "`azure-china`" + ` cloud:

    juju default-region azure-china

Unset the default region for the ` + "`azure-china`" + ` cloud:

    juju default-region azure-china --reset
`

// NewSetDefaultRegionCommand returns a command to set the default region for a cloud.
func NewSetDefaultRegionCommand() cmd.Command {
	return &setDefaultRegionCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

// SetFlags initializes the flags supported by the command.
func (c *setDefaultRegionCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.reset, "reset", false, "Reset default region for the cloud")
}

func (c *setDefaultRegionCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "default-region",
		Aliases:  []string{"set-default-region"},
		Args:     "<cloud name> [<region>]",
		Purpose:  usageSetDefaultRegionSummary,
		Doc:      usageSetDefaultRegionDetails,
		Examples: usageSetDefaultRegionnExamples,
		SeeAlso: []string{
			"add-credential",
		},
	})
}

func (c *setDefaultRegionCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("Usage: juju default-region <cloud-name> [<region>]")
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
	var cred *jujucloud.CloudCredential
	cred, err = c.store.CredentialForCloud(c.cloud)
	if errors.IsNotFound(err) {
		cred = &jujucloud.CloudCredential{}
	} else if err != nil {
		return err
	}
	if !c.reset && c.region == "" {
		// We are just reading the value.
		if cred.DefaultRegion != "" {
			ctxt.Infof("Default region for cloud %q is %q on this client.", c.cloud, cred.DefaultRegion)
			return nil
		}
		ctxt.Infof("Default region for cloud %q is not set on this client.", c.cloud)
		return nil
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
	cred.DefaultRegion = c.region
	if err := c.store.UpdateCredential(c.cloud, *cred); err != nil {
		return err
	}
	ctxt.Infof("%s", msg)
	return nil
}
