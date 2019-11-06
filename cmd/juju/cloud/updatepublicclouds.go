// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
)

type updatePublicCloudsCommand struct {
	cmd.CommandBase
	config publicCloudsConfig
}

var updatePublicCloudsDoc = `
DEPRECATED COMMAND Use 'update-cloud' instead. 

If any new information for public clouds (such as regions and connection
endpoints) are available this command will update Juju accordingly. It is
suggested to run this command periodically.

Examples:

    juju update-public-clouds

See also:
    clouds
`

// NewUpdatePublicCloudsCommand returns a command to update cloud information.
var NewUpdatePublicCloudsCommand = func() cmd.Command {
	return newUpdatePublicCloudsCommand()
}

func newUpdatePublicCloudsCommand() cmd.Command {
	return &updatePublicCloudsCommand{
		config: newPublicCloudsConfig(),
	}
}

func (c *updatePublicCloudsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "update-public-clouds",
		Aliases: []string{"update-clouds"},
		Purpose: "DEPRECATED (use 'update-cloud' instead): Updates public cloud information available to Juju.",
		Doc:     updatePublicCloudsDoc,
	})
}

func (c *updatePublicCloudsCommand) Run(ctxt *cmd.Context) error {
	newPublicClouds, err := getPublishedPublicClouds(ctxt, c.config)
	if err != nil {
		return errors.Trace(err)
	}
	currentPublicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return errors.Annotate(err, "invalid local public cloud data")
	}
	sameCloudInfo, err := jujucloud.IsSameCloudMetadata(newPublicClouds, currentPublicClouds)
	if err != nil {
		// Should never happen.
		return err
	}
	if sameCloudInfo {
		fmt.Fprintln(ctxt.Stderr, "This client's list of public clouds is up to date, see `juju clouds --client-only`.")
		return nil
	}
	if err := jujucloud.WritePublicCloudMetadata(newPublicClouds); err != nil {
		return errors.Annotate(err, "error writing new local public cloud data")
	}
	updateDetails := diffClouds(newPublicClouds, currentPublicClouds)
	fmt.Fprintln(ctxt.Stderr, fmt.Sprintf("Updated your list of public clouds with %s", updateDetails))
	return nil
}

func diffClouds(newClouds, oldClouds map[string]jujucloud.Cloud) string {
	diff := newChanges()
	// added and updated clouds
	for cloudName, cloud := range newClouds {
		oldCloud, ok := oldClouds[cloudName]
		if !ok {
			diff.addChange(addChange, cloudScope, cloudName)
			continue
		}

		if cloudChanged(cloudName, cloud, oldCloud) {
			diffCloudDetails(cloudName, cloud, oldCloud, diff)
		}
	}

	// deleted clouds
	for cloudName := range oldClouds {
		if _, ok := newClouds[cloudName]; !ok {
			diff.addChange(deleteChange, cloudScope, cloudName)
		}
	}
	return diff.summary()
}
