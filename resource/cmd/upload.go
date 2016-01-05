// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/envcmd"
)

// UploadClient has the API client methods needed by UploadCommand.
type UploadClient interface {
	Upload(service, name string, resource io.Reader) error
	Close() error
}

// UploadDeps is a type that contains external functions that Upload depends on
// to function.
type UploadDeps struct {
	// NewClient returns the value that wraps the API for uploading to the server.
	NewClient func(*UploadCommand) (UploadClient, error)
	// OpenResource handles creating a reader from the resource path.
	OpenResource func(path string) (io.ReadCloser, error)
}

// UploadCommand implements the upload command.
type UploadCommand struct {
	deps UploadDeps
	envcmd.EnvCommandBase
	service       string
	resourceFiles []resourceFile
	resources     map[string]bool
}

// NewUploadCommand returns a new command that lists resources defined
// by a charm.
func NewUploadCommand(deps UploadDeps) *UploadCommand {
	return &UploadCommand{deps: deps}
}

// Info implements cmd.Command.Info
func (c *UploadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upload",
		Args:    "service name=file [name2=file2 ...]",
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
	c.service = args[0]

	c.resources = make(map[string]bool)

	for _, arg := range args[1:] {
		if err := c.addResourceFile(arg); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// addResourceFile parses the given arg into a name and a resource file,
// and saves it in c.resourceFiles.
func (c *UploadCommand) addResourceFile(arg string) error {
	rf, err := parseResourceFile(c.service, arg)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Allow last one to win (a standard CLI approach)?
	if c.resources[rf.name] {
		msg := fmt.Sprintf("duplicate resource %q", rf.name)
		return errors.NewAlreadyExists(nil, msg)
	}
	c.resourceFiles = append(c.resourceFiles, rf)
	c.resources[rf.name] = true
	return nil
}

// Run implements cmd.Command.Run.
func (c *UploadCommand) Run(*cmd.Context) error {
	apiclient, err := c.deps.NewClient(c)
	if err != nil {
		return errors.Annotatef(err, "can't connect to %s", c.ConnectionName())
	}
	defer apiclient.Close()

	errs := []error{}

	for _, rf := range c.resourceFiles {
		// don't want to do a bulk upload since we're doing potentially large
		// file uploads.
		if err := c.upload(rf, apiclient); err != nil {
			name := rf.service + "/" + rf.name
			errs = append(errs, errors.Annotatef(err, "failed to upload resource %q", name))
		}
	}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		msgs := make([]string, len(errs))
		for i := range errs {
			msgs[i] = errs[i].Error()
		}
		return errors.Errorf(strings.Join(msgs, "\n"))
	}
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
