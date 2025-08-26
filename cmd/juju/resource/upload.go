// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/resources"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	coreresources "github.com/juju/juju/core/resources"
)

// UploadClient has the API client methods needed by UploadCommand.
type UploadClient interface {
	// Upload sends the resource to Juju.
	Upload(application, name, filename, pendingID string, resource io.ReadSeeker) error

	// ListResources returns info about resources for applications in the model.
	ListResources(applications []string) ([]coreresources.ApplicationResources, error)

	// Close closes the client.
	Close() error
}

// UploadCommand implements the upload command.
type UploadCommand struct {
	modelcmd.ModelCommandBase

	newClient func() (UploadClient, error)

	application   string
	resourceValue resourceValue
}

// NewUploadCommand returns a new command that lists resources defined
// by a charm.
func NewUploadCommand() modelcmd.ModelCommand {
	c := &UploadCommand{}
	c.newClient = func() (UploadClient, error) {
		apiRoot, err := c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return resources.NewClient(apiRoot)
	}
	return modelcmd.Wrap(c)
}

const (
	attachDoc = `
This command updates a resource for an application.

The format is

    <resource name>=<resource>

where the resource name is the name from the metadata.yaml file of the charm
and where, depending on the type of the resource, the resource can be specified
as follows:

- If the resource is type ` + "`file`" + `, you can specify it by providing one of the following:

    a. the resource revision number.

    b. a path to a local file. Caveat: If you choose this, you will not be able
	 to go back to using a resource from Charmhub.

- If the resource is type ` + "`oci-image`" + `, you can specify it by providing one of the following:

    a. the resource revision number.

	b. a path to the local file for your private OCI image as well as the
	username and password required to access the private OCI image.
	Caveat: If you choose this, you will not be able to go back to using a
	resource from Charmhub.

    c. a link to a public OCI image. Caveat: If you choose this, you will not be
	 able to go back to using a resource from Charmhub.

`
	attachExample = `
    juju attach-resource mysql resource-name=foo

    juju attach-resource ubuntu-k8s ubuntu_image=ubuntu

    juju attach-resource redis-k8s redis-image=redis
`
)

// Info implements cmd.Command.Info
func (c *UploadCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "attach-resource",
		Args:    "application <resource name>=<resource>",
		Purpose: "Update a resource for an application.",
		Doc:     attachDoc,
		SeeAlso: []string{
			"resources",
			"charm-resources",
		},
	})
}

// Init implements cmd.Command.Init. It will return an error satisfying
// errors.BadRequest if you give it an incorrect number of arguments.
func (c *UploadCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.BadRequestf("missing application name")
	case 1:
		return errors.BadRequestf("no resource specified")
	}

	c.application = args[0]
	if !names.IsValidApplication(c.application) {
		return errors.NotValidf("application %q", c.application)
	}

	if err := c.addResourceValue(args[1]); err != nil {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args[2:])
}

// addResourceValue parses the given arg into a name and a resource value,
// and saves it in c.resourceValue.
func (c *UploadCommand) addResourceValue(arg string) error {
	name, value, err := parseResourceValueArg(arg)
	if err != nil {
		return errors.Annotatef(err, "bad resource arg %q", arg)
	}
	c.resourceValue = resourceValue{
		application: c.application,
		name:        name,
		value:       value,
		// Default to file resource.
		resourceType: charmresource.TypeFile,
	}

	return nil
}

// Run implements cmd.Command.Run.
func (c *UploadCommand) Run(*cmd.Context) error {
	apiclient, err := c.newClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer apiclient.Close()

	result, err := apiclient.ListResources([]string{c.application})
	if err != nil {
		return errors.Trace(err)
	}
	resourceMeta := result[0]
	for _, r := range resourceMeta.Resources {
		if r.Name == c.resourceValue.name {
			c.resourceValue.resourceType = r.Type
		}
	}

	if err := c.upload(c.resourceValue, apiclient); err != nil {
		return errors.Annotatef(err, "failed to upload resource %q", c.resourceValue.name)
	}
	return nil
}

// upload opens the given file and calls the apiclient to upload it to the given
// application with the given name.
func (c *UploadCommand) upload(rf resourceValue, client UploadClient) error {
	f, err := OpenResource(rf.value, rf.resourceType, c.Filesystem().Open)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	err = client.Upload(rf.application, rf.name, rf.value, "", f)
	if err := block.ProcessBlockedError(err, block.BlockChange); err != nil {
		return errors.Trace(err)
	}
	return nil
}
