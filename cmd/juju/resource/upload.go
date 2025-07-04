// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/resources"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	coreresources "github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/cmd"
)

// UploadClient has the API client methods needed by UploadCommand.
type UploadClient interface {
	// Upload sends the resource to Juju.
	Upload(ctx context.Context, application, name, filename, pendingID string, resource io.ReadSeeker) error

	// ListResources returns info about resources for applications in the model.
	ListResources(ctx context.Context, applications []string) ([]coreresources.ApplicationResources, error)

	// Close closes the client.
	Close() error
}

// UploadCommand implements the upload command.
type UploadCommand struct {
	modelcmd.ModelCommandBase

	newClient func(ctx context.Context) (UploadClient, error)

	application   string
	resourceValue resourceValue
}

// NewUploadCommand returns a new command that lists resources defined
// by a charm.
func NewUploadCommand() modelcmd.ModelCommand {
	c := &UploadCommand{}
	c.newClient = func(ctx context.Context) (UploadClient, error) {
		apiRoot, err := c.NewAPIRoot(ctx)
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

(1) If the resource is type 'file', you can specify it by providing
(a) the resource revision number or
(b) a path to a local file.

(2) If the resource is type 'oci-image', you can specify it by providing
(a) the resource revision number,
(b) a path to a local file = private OCI image,
(c) a link to a public OCI image.


Note: If you choose (1b) or (2b-c), i.e., a resource that is not from Charmhub:
You will not be able to go back to using a resource from Charmhub.

Note: If you choose (1b) or (2b): This uploads a file from your local disk to the juju
controller to be streamed to the charm when "resource-get" is called by a hook.

Note: If you choose (2b): You will need to specify:
(i) the local path to the private OCI image as well as
(ii) the username/password required to access the private OCI image.

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
func (c *UploadCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.newClient(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer apiclient.Close()

	result, err := apiclient.ListResources(ctx, []string{c.application})
	if err != nil {
		return errors.Trace(err)
	}
	resourceMeta := result[0]
	for _, r := range resourceMeta.Resources {
		if r.Name == c.resourceValue.name {
			c.resourceValue.resourceType = r.Type
		}
	}

	if err := c.upload(ctx, c.resourceValue, apiclient); err != nil {
		return errors.Annotatef(err, "failed to upload resource %q", c.resourceValue.name)
	}
	return nil
}

// upload opens the given file and calls the apiclient to upload it to the given
// application with the given name.
func (c *UploadCommand) upload(ctx context.Context, rf resourceValue, client UploadClient) error {
	f, err := OpenResource(rf.value, rf.resourceType, c.Filesystem().Open)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	err = client.Upload(ctx, rf.application, rf.name, rf.value, "", f)
	if err := block.ProcessBlockedError(err, block.BlockChange); err != nil {
		return errors.Trace(err)
	}
	return nil
}
