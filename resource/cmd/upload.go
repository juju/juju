// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

// UploadClient has the API client methods needed by UploadCommand.
type UploadClient interface {
	// Upload sends the resource to Juju.
	Upload(service, name string, resource io.ReadSeeker) error

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
	service      string
	resourceFile resourceFile
}

// NewUploadCommand returns a new command that lists resources defined
// by a charm.
func NewUploadCommand(deps UploadDeps) *UploadCommand {
	return &UploadCommand{deps: deps}
}

// Info implements cmd.Command.Info
func (c *UploadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "push-resource",
		Args:    "service name=file",
		Purpose: "upload a file as a resource for a service",
		Doc: `
This command uploads a file from your local disk to the juju controller to be
used as a resource for a service.
`,
	}
}

// Init implements cmd.Command.Init. It will return an error satisfying
// errors.BadRequest if you give it an incorrect number of arguments.
func (c *UploadCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.BadRequestf("missing service name")
	case 1:
		return errors.BadRequestf("no resource specified")
	}

	service := args[0]
	if service == "" { // TODO(ericsnow) names.IsValidService
		return errors.NewNotValid(nil, "missing service name")
	}
	c.service = service

	if err := c.addResourceFile(args[1]); err != nil {
		return errors.Trace(err)
	}
	if err := cmd.CheckEmpty(args[2:]); err != nil {
		return errors.NewBadRequest(err, "")
	}

	return nil
}

// addResourceFile parses the given arg into a name and a resource file,
// and saves it in c.resourceFiles.
func (c *UploadCommand) addResourceFile(arg string) error {
	name, filename, err := parseResourceFileArg(arg)
	if err != nil {
		return errors.Annotatef(err, "bad resource arg %q", arg)
	}
	c.resourceFile = resourceFile{
		service:  c.service,
		name:     name,
		filename: filename,
	}

	return nil
}

// Run implements cmd.Command.Run.
func (c *UploadCommand) Run(*cmd.Context) error {
	apiclient, err := c.deps.NewClient(c)
	if err != nil {
		return errors.Annotatef(err, "can't connect to %s", c.ConnectionName())
	}
	defer apiclient.Close()

	if err := c.upload(c.resourceFile, apiclient); err != nil {
		return errors.Annotatef(err, "failed to upload resource %q", c.resourceFile.name)
	}
	return nil
}

// upload opens the given file and calls the apiclient to upload it to the given
// service with the given name.
func (c *UploadCommand) upload(rf resourceFile, client UploadClient) error {
	f, err := c.deps.OpenResource(rf.filename)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	err = client.Upload(rf.service, rf.name, f)
	return errors.Trace(err)
}
