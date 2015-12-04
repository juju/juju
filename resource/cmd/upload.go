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
	NewClient    func(c *UploadCommand) (UploadClient, error)
	OpenResource func(path string) (io.ReadCloser, error)
}

// UploadCommand implements the upload command.
type UploadCommand struct {
	UploadDeps
	envcmd.EnvCommandBase
	service   string
	resources map[string]string
}

// NewUploadCommand returns a new command that lists resources defined
// by a charm.
func NewUploadCommand(deps UploadDeps) *UploadCommand {
	return &UploadCommand{UploadDeps: deps}
}

const uploadDoc = `
This command uploads a file from your local disk to the juju controller to be
used as a resource for a service.
`

func (c *UploadCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upload",
		Args:    "service name=file [name2=file2 ...]",
		Purpose: "upload a file as a resource for a service",
		Doc:     uploadDoc,
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

	c.resources = make(map[string]string, len(args)-1)

	for _, arg := range args[1:] {
		if err := c.addResource(arg); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// addResource parses the given arg into a name and a resource file, and saves
// it in c.resources.
func (c *UploadCommand) addResource(arg string) error {
	vals := strings.SplitN(arg, "=", 2)
	if len(vals) < 2 || vals[0] == "" || vals[1] == "" {
		return errors.NotValidf("resource %q", arg)
	}
	name := vals[0]
	if _, ok := c.resources[name]; ok {
		return errors.AlreadyExistsf("resource %q", name)
	}
	c.resources[name] = vals[1]
	return nil
}

// Run implements cmd.Command.Run.
func (c *UploadCommand) Run(*cmd.Context) error {
	apiclient, err := c.NewClient(c)
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()

	for name, file := range c.resources {
		if err := c.upload(c.service, name, file, apiclient); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// upload opens the given file and calls the apiclient to upload it to the given
// service with the given name.
func (c *UploadCommand) upload(service, name, file string, client UploadClient) error {
	f, err := c.OpenResource(file)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	return client.Upload(service, name, f)
}
