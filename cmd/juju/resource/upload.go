// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
)

// UploadClient has the API client methods needed by UploadCommand.
type UploadClient interface {
	// Upload sends the resource to Juju.
	Upload(application, name, filename string, resource io.ReadSeeker) error

	// ListResources returns info about resources for applications in the model.
	ListResources(applications []string) ([]resource.ApplicationResources, error)

	// Close closes the client.
	Close() error
}

// ReadSeekCloser combines 2 interfaces.
type ReadSeekCloser interface {
	io.ReadCloser
	io.Seeker
}

// UploadDeps is a type that contains external functions that Upload depends on
// to function.
type UploadDeps struct {
	// NewClient returns the value that wraps the API for uploading to the server.
	NewClient func(*UploadCommand) (UploadClient, error)

	// OpenResource handles creating a reader from the resource path.
	OpenResource func(path string) (ReadSeekCloser, error)
}

// UploadCommand implements the upload command.
type UploadCommand struct {
	deps UploadDeps
	modelcmd.ModelCommandBase
	application   string
	resourceValue resourceValue
}

// NewUploadCommand returns a new command that lists resources defined
// by a charm.
func NewUploadCommand(deps UploadDeps) modelcmd.ModelCommand {
	return modelcmd.Wrap(&UploadCommand{deps: deps})
}

const (
	attachDoc = `
This command updates a resource for an application.

For file resources, it uploads a file from your local disk to the juju controller to be
streamed to the charm when "resource-get" is called by a hook.

For OCI image resources used by k8s applications, an OCI image or file path is specified.
A file is specified when a private OCI image is needed and the username/password used to
access the image is needed along with the image path.
`
)

// Info implements cmd.Command.Info
func (c *UploadCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "attach-resource",
		Args:    "application name=file|OCI image",
		Purpose: "Update a resource for an application.",
		Doc:     attachDoc,
		Aliases: []string{"attach"},
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
	apiclient, err := c.deps.NewClient(c)
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
	f, err := OpenResource(rf.value, rf.resourceType, c.deps.OpenResource)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	err = client.Upload(rf.application, rf.name, rf.value, f)
	if err := block.ProcessBlockedError(err, block.BlockChange); err != nil {
		return errors.Trace(err)
	}
	return nil
}
