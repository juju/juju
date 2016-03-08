// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"

	jujucloud "github.com/juju/juju/cloud"
)

type updateCloudsCommand struct {
	cmd.CommandBase

	publicCloudURL string
}

var updateCloudsDoc = `
The update-clouds command updates the public cloud information available to Juju.
If any new regions or updated endpoints are available, this command will ensure Juju
knows about that information when bootstrapping a new controller.

Example:
   juju update-clouds
`

// NewUpdateCloudsCommand returns a command to update cloud information.
func NewUpdateCloudsCommand() cmd.Command {
	return &updateCloudsCommand{
		publicCloudURL: "https://streams.canonical.com/juju/public-clouds.yaml",
	}
}

func (c *updateCloudsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update-clouds",
		Purpose: "update public cloud regions and endpoints",
		Doc:     updateCloudsDoc,
	}
}

func (c *updateCloudsCommand) Run(ctxt *cmd.Context) error {
	fmt.Fprint(ctxt.Stdout, "Fetching latest public cloud list... ")
	client := utils.GetHTTPClient(utils.VerifySSLHostnames)
	resp, err := client.Get(c.publicCloudURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	noNewClouds := "\nno new public cloud information available at this time"
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			fmt.Fprintln(ctxt.Stdout, noNewClouds)
			return nil
		case http.StatusUnauthorized:
			return errors.Unauthorizedf("unauthorised access to URL %q", c.publicCloudURL)
		}
		return fmt.Errorf("cannot read public cloud information at URL %q, %q", c.publicCloudURL, resp.Status)
	}

	cloudData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Annotate(err, "error receiving updated cloud data")
	}
	newPublicClouds, err := jujucloud.ParseCloudMetadata(cloudData)
	if err != nil {
		return errors.Annotate(err, "invalid cloud data received when updating clouds")
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
		fmt.Fprintln(ctxt.Stdout, noNewClouds)
		return nil
	}
	if err := jujucloud.WritePublicCloudMetadata(newPublicClouds); err != nil {
		return errors.Annotate(err, "error writing new local public cloud data")
	}
	fmt.Fprintln(ctxt.Stdout, "done.")
	return nil
}
